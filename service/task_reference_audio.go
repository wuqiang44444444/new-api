package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Transport bounds only: do not inspect duration, sample rate, or provider limits.
const MaxVideoReferenceAudioBytes = 15 << 20
const referenceAudioContextKey = "seedance_reference_audio"
const referenceAudioInputsContextKey = "seedance_reference_audio_inputs"

// VideoReferenceAudioInput is request-local transport data, never a Task field.
// Source binds the decoded bytes to the exact content item accepted by ingress.
type VideoReferenceAudioInput struct {
	Source   string
	Data     []byte
	MimeType string
}

func SetVideoReferenceAudioInputs(c *gin.Context, inputs map[int]VideoReferenceAudioInput) {
	c.Set(referenceAudioInputsContextKey, inputs)
}

// DecodeVideoReferenceAudio accepts an audio Data URL or bare Base64 file bytes.
// HTTP(S) and opaque asset references remain untouched and are never downloaded.
func DecodeVideoReferenceAudio(source string) ([]byte, string, error) {
	value := strings.TrimSpace(source)
	if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "asset://") {
		return nil, "", nil
	}
	contentType := ""
	encoded := value
	if strings.HasPrefix(value, "data:") {
		header, payload, ok := strings.Cut(value[5:], ",")
		if !ok || !strings.HasSuffix(strings.ToLower(header), ";base64") {
			return nil, "", errors.New("reference audio must contain Base64 file bytes")
		}
		var err error
		contentType, _, err = mime.ParseMediaType(header[:len(header)-len(";base64")])
		if err != nil {
			return nil, "", errors.New("reference audio has an invalid MIME type")
		}
		encoded = payload
	}
	data, err := io.ReadAll(io.LimitReader(base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(encoded)), MaxVideoReferenceAudioBytes+1))
	if err != nil || len(data) == 0 || len(data) > MaxVideoReferenceAudioBytes {
		return nil, "", errors.New("reference audio contains invalid or oversized Base64 file bytes")
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	return data, contentType, nil
}

// PrepareVideoReferenceAudio uploads before the durable funding hold. Sources
// stay in request memory; only object facts enter Task/attempt snapshots.
func PrepareVideoReferenceAudio(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if _, exists := c.Get(referenceAudioContextKey); exists {
		return nil
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil {
		return nil
	}
	prepared := make([]model.TaskReferenceAudioFact, 0)
	value, _ := c.Get(referenceAudioInputsContextKey)
	inputs, _ := value.(map[int]VideoReferenceAudioInput)
	urls := map[int]string{}
	ctx := c.Request.Context()
	type hostedAssetPresigner interface {
		presignHostedAssetURL(string, time.Duration) (string, error)
	}
	var location model.FunCloudHostedStorageLocation
	var presigner hostedAssetPresigner
	for index, item := range contract.ModelArk.Content {
		if item.Type != "audio_url" || item.AudioURL == nil {
			continue
		}
		value := strings.TrimSpace(item.AudioURL.URL)
		if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "asset://") {
			continue
		}
		input, exists := inputs[index]
		if !exists || input.Source != item.AudioURL.URL || len(input.Data) == 0 || len(input.Data) > MaxVideoReferenceAudioBytes {
			return TaskErrorWrapperLocal(errors.New("reference audio input was not prepared by ingress"), "invalid_video_parameter", http.StatusBadRequest)
		}
		data, contentType := input.Data, input.MimeType
		if info == nil || info.UserId <= 0 {
			return TaskErrorWrapperLocal(errors.New("reference audio owner is unavailable"), "internal_error", http.StatusInternalServerError)
		}
		// Store, location, and presigner are request-frozen configuration:
		// resolve them once on the first inline item and reuse for the rest.
		if presigner == nil {
			var err error
			ctx, err = WithImageObjectStore(ctx)
			if err != nil {
				return TaskErrorWrapperLocal(errors.New("reference audio storage is unavailable"), "reference_audio_unavailable", http.StatusServiceUnavailable)
			}
			location, err = funCloudHostedStorageLocation(ctx)
			if err != nil {
				return TaskErrorWrapperLocal(errors.New("reference audio storage is unavailable"), "reference_audio_unavailable", http.StatusServiceUnavailable)
			}
			session, err := imageObjectSessionForContext(ctx)
			if err != nil {
				return TaskErrorWrapperLocal(errors.New("reference audio storage is unavailable"), "reference_audio_unavailable", http.StatusServiceUnavailable)
			}
			store, ok := session.store.(hostedAssetPresigner)
			if !ok {
				return TaskErrorWrapperLocal(errors.New("reference audio storage does not support signed URLs"), "reference_audio_unavailable", http.StatusServiceUnavailable)
			}
			presigner = store
		}
		random, err := common.GenerateRandomCharsKey(24)
		if err != nil {
			return TaskErrorWrapperLocal(errors.New("reference audio object could not be allocated"), "internal_error", http.StatusInternalServerError)
		}
		var extension string
		switch contentType {
		case "audio/wav", "audio/wave", "audio/x-wav":
			extension = ".wav"
		case "audio/mpeg":
			extension = ".mp3"
		case "audio/ogg":
			extension = ".ogg"
		case "audio/mp4":
			extension = ".m4a"
		}
		appID := 0
		if info.TaskRelayInfo != nil {
			appID = info.TaskRelayInfo.AppID
		}
		objectKey := fmt.Sprintf("media/seedance/%d/%d/audio-%s%s", info.UserId, appID, random, extension)
		if _, err := PutImageObject(ctx, objectKey, contentType, data); err != nil {
			return TaskErrorWrapperLocal(errors.New("reference audio upload failed"), "reference_audio_unavailable", http.StatusServiceUnavailable)
		}
		signed, err := presigner.presignHostedAssetURL(objectKey, 24*time.Hour)
		parsed, parseErr := url.Parse(signed)
		if err != nil || parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return TaskErrorWrapperLocal(errors.New("reference audio URL could not be signed"), "reference_audio_unavailable", http.StatusServiceUnavailable)
		}
		urls[index] = signed
		prepared = append(prepared, model.TaskReferenceAudioFact{
			ContentIndex: index, ObjectKey: objectKey, StorageLocation: location, MimeType: contentType, SizeBytes: int64(len(data)),
		})
	}
	for index, signed := range urls {
		contract.ModelArk.Content[index].AudioURL.URL = signed
	}
	// Release decoded bytes once the request contains only upstream-ready URLs.
	c.Set(referenceAudioInputsContextKey, nil)

	c.Request = c.Request.WithContext(ctx)
	c.Set(referenceAudioContextKey, prepared)
	return nil
}

func StageVideoReferenceAudioSnapshot(c *gin.Context, task *model.Task) {
	if c == nil || task == nil {
		return
	}
	value, _ := c.Get(referenceAudioContextKey)
	prepared, _ := value.([]model.TaskReferenceAudioFact)
	task.PrivateData.ReferenceAudio = append([]model.TaskReferenceAudioFact(nil), prepared...)
}
