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
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type modelArkVideoMediaURL struct {
	URL string `json:"url"`
}

type modelArkVideoContent struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Role     string                 `json:"role,omitempty"`
	ImageURL *modelArkVideoMediaURL `json:"image_url,omitempty"`
	VideoURL *modelArkVideoMediaURL `json:"video_url,omitempty"`
	AudioURL *modelArkVideoMediaURL `json:"audio_url,omitempty"`
}

type modelArkVideoCreateRequest struct {
	Model         string                 `json:"model"`
	Content       []modelArkVideoContent `json:"content"`
	CallbackURL   string                 `json:"callback_url,omitempty"`
	Duration      *int                   `json:"duration,omitempty"`
	Resolution    *string                `json:"resolution,omitempty"`
	Ratio         *string                `json:"ratio,omitempty"`
	ServiceTier   *string                `json:"service_tier,omitempty"`
	GenerateAudio *bool                  `json:"generate_audio,omitempty"`
	Watermark     *bool                  `json:"watermark,omitempty"`
}

func ModelArkVideoCreateConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request modelArkVideoCreateRequest
		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", "invalid request body")
			return
		}
		encoded, err := common.Marshal(body)
		if err != nil || common.Unmarshal(encoded, &request) != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", "invalid request body")
			return
		}
		if strings.TrimSpace(request.Model) == "" || len(request.Content) == 0 {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", "model and content are required")
			return
		}
		if strings.TrimSpace(request.CallbackURL) != "" {
			abortModelArkVideo(c, http.StatusBadRequest, "unsupported_parameter", "callback_url is not supported; poll the task API")
			return
		}
		prompt, err := validateModelArkVideoCreateRequest(request)
		if err != nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		delete(body, "model")
		delete(body, "content")
		delete(body, "callback_url")
		body["content"] = request.Content
		internalBody, err := common.Marshal(map[string]any{
			"model":    request.Model,
			"prompt":   prompt,
			"metadata": body,
		})
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

func validateModelArkVideoCreateRequest(request modelArkVideoCreateRequest) (string, error) {
	if strings.TrimSpace(request.Model) == "" || len(request.Content) == 0 {
		return "", fmt.Errorf("model and content are required")
	}
	if request.Duration != nil && (*request.Duration != -1 && (*request.Duration < 4 || *request.Duration > 15)) {
		return "", fmt.Errorf("duration must be -1 or between 4 and 15")
	}
	if request.Resolution != nil && !modelArkVideoValueAllowed(*request.Resolution, "480p", "720p", "1080p", "4k") {
		return "", fmt.Errorf("resolution is not supported")
	}
	if request.Ratio != nil && !modelArkVideoValueAllowed(*request.Ratio, "16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive") {
		return "", fmt.Errorf("ratio is not supported")
	}
	if request.ServiceTier != nil && !modelArkVideoValueAllowed(*request.ServiceTier, "default", "flex") {
		return "", fmt.Errorf("service_tier is not supported")
	}

	texts := make([]string, 0, len(request.Content))
	imageCount, videoCount, audioCount := 0, 0, 0
	roleCounts := make(map[string]int)
	for index, item := range request.Content {
		switch item.Type {
		case "text":
			text := strings.TrimSpace(item.Text)
			if text == "" || item.ImageURL != nil || item.VideoURL != nil || item.AudioURL != nil || strings.TrimSpace(item.Role) != "" {
				return "", fmt.Errorf("content[%d] is not a valid text item", index)
			}
			texts = append(texts, text)
		case "image_url":
			imageCount++
			if err := validateModelArkVideoMediaItem(index, item, item.ImageURL, "first_frame", "last_frame", "reference_image"); err != nil {
				return "", err
			}
			roleCounts[strings.TrimSpace(item.Role)]++
		case "video_url":
			videoCount++
			if err := validateModelArkVideoMediaItem(index, item, item.VideoURL, "reference_video"); err != nil {
				return "", err
			}
			roleCounts[strings.TrimSpace(item.Role)]++
		case "audio_url":
			audioCount++
			if err := validateModelArkVideoMediaItem(index, item, item.AudioURL, "reference_audio"); err != nil {
				return "", err
			}
			roleCounts[strings.TrimSpace(item.Role)]++
		default:
			return "", fmt.Errorf("content[%d].type is not supported", index)
		}
	}
	if imageCount > 9 || videoCount > 3 || audioCount > 3 {
		return "", fmt.Errorf("content exceeds the supported media count")
	}
	if roleCounts["first_frame"] > 1 || roleCounts["last_frame"] > 1 {
		return "", fmt.Errorf("content supports at most one first_frame and one last_frame")
	}
	if roleCounts["last_frame"] > 0 && roleCounts["first_frame"] == 0 {
		return "", fmt.Errorf("last_frame requires first_frame")
	}
	if audioCount > 0 && imageCount == 0 && videoCount == 0 {
		return "", fmt.Errorf("audio input requires an image or video reference")
	}
	return strings.Join(texts, "\n"), nil
}

func validateModelArkVideoMediaItem(index int, item modelArkVideoContent, media *modelArkVideoMediaURL, roles ...string) error {
	if media == nil || item.Text != "" {
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
	role := strings.TrimSpace(item.Role)
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

func ModelArkVideoChannelConstraint() gin.HandlerFunc {
	return func(c *gin.Context) {
		var channels []model.Channel
		if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeDoubaoVideo, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "video service is temporarily unavailable")
			return
		}
		allowed := make(map[int]struct{})
		for i := range channels {
			settings := channels[i].GetOtherSettings()
			if modelArkVideoProfileCompatible(settings.VideoUpstreamProfile) {
				allowed[channels[i].Id] = struct{}{}
			}
		}
		if existingValue, ok := common.GetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs); ok {
			existing, typeOK := existingValue.(map[int]struct{})
			if !typeOK {
				allowed = nil
			} else {
				for channelID := range allowed {
					if _, exists := existing[channelID]; !exists {
						delete(allowed, channelID)
					}
				}
			}
		}
		if len(allowed) == 0 {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "no compatible ModelArk video channel is available")
			return
		}
		common.SetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs, allowed)
		c.Next()
	}
}

// modelArkVideoProfileCompatible reports whether the ModelArk northbound
// protocol can use a DoubaoVideo channel's configured southbound protocol.
func modelArkVideoProfileCompatible(profile dto.VideoUpstreamProfile) bool {
	return profile == "" || profile.IsValid()
}

func abortModelArkVideo(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"code":       code,
		"message":    message,
		"request_id": c.GetString(common.RequestIdKey),
	}})
}
