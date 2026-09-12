package middleware

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// JSON keeps the published ModelArk shape. Multipart carries that same JSON in
// one request part and audio attachments referenced by file://<part-name>.
// File references never access local paths or remote file APIs.
func readModelArkVideoBody(c *gin.Context, body *map[string]any) error {
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil {
		return errors.New("invalid content type")
	}
	files := map[string][]byte{}
	types := map[string]string{}
	if mediaType == "multipart/form-data" {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return err
		}
		if _, err = storage.Seek(0, io.SeekStart); err != nil {
			return err
		}
		reader := multipart.NewReader(storage, params["boundary"])
		seen := map[string]bool{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return errors.New("invalid multipart request")
			}
			name := part.FormName()
			if name == "" || seen[name] {
				return errors.New("multipart fields must have unique nonempty names")
			}
			seen[name] = true
			var reader io.Reader = part
			if name != "request" {
				reader = io.LimitReader(part, service.MaxVideoReferenceAudioBytes+1)
			}
			data, err := io.ReadAll(reader)
			_ = part.Close()
			if err != nil || (name != "request" && len(data) > service.MaxVideoReferenceAudioBytes) {
				return errors.New("multipart part exceeds the transport size limit")
			}
			if name == "request" {
				if part.FileName() != "" || common.Unmarshal(data, body) != nil {
					return errors.New("request must be a ModelArk JSON form field")
				}
				continue
			}
			if part.FileName() == "" || len(data) == 0 {
				return errors.New("only request and audio file parts are accepted")
			}
			contentType := part.Header.Get("Content-Type")
			if contentType == "" || contentType == "application/octet-stream" {
				contentType = http.DetectContentType(data)
			}
			contentType, _, err = mime.ParseMediaType(contentType)
			if err != nil {
				return errors.New("audio file part has an invalid MIME type")
			}
			files[name], types[name] = data, contentType
		}
		if !seen["request"] {
			return errors.New("multipart request requires a request JSON field")
		}
	} else if mediaType == "application/json" {
		if err := common.UnmarshalBodyReusable(c, body); err != nil {
			return err
		}
	} else {
		return errors.New("expected application/json or multipart/form-data")
	}
	content, _ := (*body)["content"].([]any)
	used := map[string]bool{}
	inputs := map[int]service.VideoReferenceAudioInput{}
	for index, value := range content {
		item, _ := value.(map[string]any)
		if item["type"] != "audio_url" {
			continue
		}
		media, _ := item["audio_url"].(map[string]any)
		ref, ok := media["url"].(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(ref, "file://") {
			name := strings.TrimPrefix(ref, "file://")
			data, exists := files[name]
			if !exists {
				return errors.New("audio file reference does not match an uploaded part")
			}
			inputs[index] = service.VideoReferenceAudioInput{Source: ref, Data: data, MimeType: types[name]}
			used[name] = true
		} else {
			data, contentType, err := service.DecodeVideoReferenceAudio(ref)
			if err != nil {
				return err
			}
			if data != nil {
				inputs[index] = service.VideoReferenceAudioInput{Source: ref, Data: data, MimeType: contentType}
			}
		}
	}
	if len(used) != len(files) {
		return errors.New("each uploaded audio file must be referenced in content")
	}
	service.SetVideoReferenceAudioInputs(c, inputs)
	return nil
}
