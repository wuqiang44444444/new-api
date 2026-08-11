package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type modelArkVideoMediaURL = dto.VideoMediaURL
type modelArkVideoContent = dto.ModelArkVideoContent
type modelArkVideoCreateRequest = dto.ModelArkVideoCreateRequest

func ModelArkVideoCreateConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request modelArkVideoCreateRequest
		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", "invalid request body")
			return
		}
		if err := rejectUnknownVideoFields(
			body,
			"model", "content", "callback_url", "duration", "resolution", "ratio",
			"service_tier", "generate_audio", "watermark", "return_last_frame",
			"execution_expires_after", "draft", "tools", "safety_identifier",
			"priority", "frames", "seed", "camera_fixed",
		); err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "unsupported_parameter", err.Error())
			return
		}
		if err := rejectUnknownModelArkVideoFields(body); err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "unsupported_parameter", err.Error())
			return
		}
		if err := decodeTypedVideoRequest(body, &request); err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", "invalid request body")
			return
		}
		if strings.TrimSpace(request.Model) == "" || len(request.Content) == 0 {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", "model and content are required")
			return
		}
		appID := c.GetInt("token_id")
		common.SetContextKey(c, constant.ContextKeyAssetAppID, appID)
		prompt, err := validateModelArkVideoCreateRequest(request)
		if err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk:   &request,
		})
		internalRequest := relaycommon.TaskSubmitReq{Model: request.Model, Prompt: prompt}
		if request.Duration != nil {
			internalRequest.Duration = *request.Duration
		}
		internalBody, err := common.Marshal(internalRequest)
		if err != nil {
			abortModelArkVideo(c, http.StatusInternalServerError, "internal_error", "failed to prepare request")
			return
		}
		oldStorage, _ := common.GetBodyStorage(c)
		newStorage, err := common.CreateBodyStorage(internalBody)
		if err != nil {
			abortModelArkVideo(c, http.StatusInternalServerError, "internal_error", "failed to prepare request")
			return
		}
		if oldStorage != nil {
			_ = oldStorage.Close()
		}
		c.Set(common.KeyBodyStorage, newStorage)
		common.SetContextKey(c, constant.ContextKeyTaskPromptValidated, true)
		common.SetContextKey(c, constant.ContextKeyTaskDurationValidated, true)
		c.Request.Body = io.NopCloser(common.ReaderOnly(newStorage))
		c.Request.ContentLength = int64(len(internalBody))
		c.Request.Header.Set("Content-Type", "application/json")
		originalPath := c.Request.URL.Path
		c.Request.URL.Path = "/v1/video/generations"
		c.Next()
		c.Request.URL.Path = originalPath
	}
}

func rejectUnknownModelArkVideoFields(body map[string]any) error {
	content, exists := body["content"]
	if exists {
		if err := rejectUnknownNestedVideoArrayFields(
			content,
			"content",
			"type", "text", "role", "image_url", "video_url", "audio_url",
		); err != nil {
			return err
		}
		for index, rawItem := range content.([]any) {
			item := rawItem.(map[string]any)
			for _, mediaField := range []string{"image_url", "video_url", "audio_url"} {
				if media, ok := item[mediaField]; ok {
					if err := rejectUnknownNestedVideoFields(media, fmt.Sprintf("content[%d].%s", index, mediaField), "url"); err != nil {
						return err
					}
				}
			}
		}
	}
	if tools, exists := body["tools"]; exists {
		if err := rejectUnknownNestedVideoArrayFields(tools, "tools", "type"); err != nil {
			return err
		}
	}
	return nil
}

func validateModelArkVideoCreateRequest(request modelArkVideoCreateRequest) (string, error) {
	if strings.TrimSpace(request.Model) == "" || len(request.Content) == 0 {
		return "", fmt.Errorf("model and content are required")
	}
	if request.Duration != nil && (*request.Duration == 0 || *request.Duration < -1 || *request.Duration > relaycommon.MaxTaskDurationSeconds) {
		return "", fmt.Errorf("duration must be -1 or between 1 and %d", relaycommon.MaxTaskDurationSeconds)
	}
	if request.Frames != nil && (*request.Frames < 29 || *request.Frames > 289 || (*request.Frames-25)%4 != 0) {
		return "", fmt.Errorf("frames must be between 29 and 289 and match 25 + 4n")
	}
	if request.ExecutionExpiresAfter != nil && (*request.ExecutionExpiresAfter < 3600 || *request.ExecutionExpiresAfter > 259200) {
		return "", fmt.Errorf("execution_expires_after must be between 3600 and 259200")
	}
	if request.Priority != nil && (*request.Priority < 0 || *request.Priority > 9) {
		return "", fmt.Errorf("priority must be between 0 and 9")
	}
	if request.Seed != nil && (*request.Seed < -1 || int64(*request.Seed) > int64(1<<31-1)) {
		return "", fmt.Errorf("seed must be between -1 and 2147483647")
	}
	texts := make([]string, 0, len(request.Content))
	for index, item := range request.Content {
		switch item.Type {
		case "text":
			text := strings.TrimSpace(videoStringValue(item.Text))
			if text == "" || item.ImageURL != nil || item.VideoURL != nil || item.AudioURL != nil || strings.TrimSpace(videoStringValue(item.Role)) != "" {
				return "", fmt.Errorf("content[%d] is not a valid text item", index)
			}
			texts = append(texts, text)
		case "image_url":
			if err := validateModelArkVideoMediaItem(index, item, item.ImageURL, "first_frame", "last_frame", "reference_image"); err != nil {
				return "", err
			}
		case "video_url":
			if err := validateModelArkVideoMediaItem(index, item, item.VideoURL, "reference_video"); err != nil {
				return "", err
			}
		case "audio_url":
			if err := validateModelArkVideoMediaItem(index, item, item.AudioURL, "reference_audio"); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("content[%d].type is not supported", index)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func validateModelArkVideoMediaItem(index int, item modelArkVideoContent, media *modelArkVideoMediaURL, roles ...string) error {
	if media == nil || videoStringValue(item.Text) != "" {
		return fmt.Errorf("content[%d] has an invalid media payload", index)
	}
	if item.Type != "image_url" && item.ImageURL != nil ||
		item.Type != "video_url" && item.VideoURL != nil ||
		item.Type != "audio_url" && item.AudioURL != nil {
		return fmt.Errorf("content[%d] has multiple media payloads", index)
	}
	if err := validateModelArkVideoMediaURL(media.URL); err != nil {
		return fmt.Errorf("content[%d]: %w", index, err)
	}
	role := strings.TrimSpace(videoStringValue(item.Role))
	if role == "" || !modelArkVideoValueAllowed(role, roles...) {
		return fmt.Errorf("content[%d].role is not supported", index)
	}
	return nil
}

func validateModelArkVideoMediaURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 20*1024*1024 {
		return fmt.Errorf("media URL is invalid")
	}
	if strings.HasPrefix(value, "asset://") {
		if strings.TrimSpace(strings.TrimPrefix(value, "asset://")) == "" {
			return fmt.Errorf("asset URL is invalid")
		}
		return nil
	}
	if strings.HasPrefix(value, "data:") {
		if !strings.Contains(value, ",") {
			return fmt.Errorf("data URL is invalid")
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return fmt.Errorf("media URL must be an http(s), data, or asset URL")
	}
	return nil
}

func modelArkVideoValueAllowed(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func abortModelArkVideo(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"code":       code,
		"message":    message,
		"request_id": c.GetString(common.RequestIdKey),
	}})
}
