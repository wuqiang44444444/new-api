package seedance

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func validateSynlinkRequest(providerModel string, request *dto.ModelArkVideoCreateRequest) error {
	if !slices.Contains(kitdto.SynlinkVideoModels(), providerModel) || request == nil {
		return fmt.Errorf("the selected model is not registered for this video protocol")
	}
	// Only the documented create fields are published for this new protocol.
	// Callback delivery is not yet verified; forwarding Provider identities to
	// the caller would violate the existing northbound task contract.
	if request.CallbackURL != nil || request.ServiceTier != nil || request.ExecutionExpiresAfter != nil || request.Draft != nil || request.Tools != nil || request.SafetyIdentifier != nil || request.Priority != nil || request.Frames != nil || request.Seed != nil || request.CameraFixed != nil || request.OutputFormat != nil {
		return fmt.Errorf("request contains a parameter not published for this video protocol")
	}
	if request.Duration != nil && (*request.Duration < 1 || *request.Duration > relaycommon.MaxTaskDurationSeconds) {
		return fmt.Errorf("duration must be between 1 and %d; automatic duration is not verified", relaycommon.MaxTaskDurationSeconds)
	}
	if request.Resolution != nil && !slices.Contains([]string{"480p", "720p", "1080p", "4k", "4K"}, *request.Resolution) {
		return fmt.Errorf("resolution is outside the published video request format")
	}
	if request.Ratio != nil {
		if _, ok := modelArkRatios[*request.Ratio]; !ok {
			return fmt.Errorf("ratio is outside the published video request format")
		}
	}
	for _, item := range request.Content {
		for _, media := range []*dto.VideoMediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			ref := strings.TrimSpace(media.URL)
			if strings.HasPrefix(ref, "asset://"+model.FunCloudHostedAssetIDPrefix) && item.Type == "image_url" && media == item.ImageURL {
				continue
			}
			// No documented prohibition of audio/video data URLs. Opaque asset
			// references still require the registered hosted-image conversion.
			if (item.Type == "audio_url" || item.Type == "video_url") && strings.HasPrefix(ref, "data:") && strings.IndexByte(ref, ',') > len("data:") {
				continue
			}
			parsed, err := url.Parse(ref)
			if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return fmt.Errorf("this video protocol requires a platform-hosted image or an HTTP(S) media URL")
			}
		}
	}
	return nil
}

func buildSynlinkRequest(c *gin.Context, body *requestPayload) ([]byte, error) {
	payload := *body
	// Pin northbound defaults so the frozen billing probe matches the wire.
	if payload.Duration == nil {
		value := kitdto.IntValue(5)
		payload.Duration = &value
	}
	if payload.Resolution == "" {
		payload.Resolution = "720p"
	}
	if payload.GenerateAudio == nil {
		value := kitdto.BoolValue(false)
		payload.GenerateAudio = &value
	}
	var err error
	payload.Content, err = resolveHostedImageContent(c, payload.Content)
	if err != nil {
		return nil, err
	}
	for _, item := range payload.Content {
		for _, media := range []*MediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media != nil && strings.HasPrefix(strings.TrimSpace(media.URL), "asset://") {
				return nil, fmt.Errorf("unresolved asset reference cannot be sent to this video protocol")
			}
		}
	}
	return common.Marshal(&payload)
}

// attachSynlinkLastFrame publishes the internal content marker only when the
// accepted request asked for a last frame. No upstream URL or key is public.
func attachSynlinkLastFrame(task *model.Task, body []byte) ([]byte, error) {
	request := task.PrivateData.ClientRequest
	if request == nil || request.ReturnLastFrame == nil || !*request.ReturnLastFrame {
		return body, nil
	}
	var result map[string]any
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result["status"] != "succeeded" {
		return body, nil
	}
	content, ok := result["content"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("completed task has no content")
	}
	content["last_frame_url"] = kitdto.SynlinkLastFramePath(task.GetUpstreamTaskID())
	return common.Marshal(result)
}
