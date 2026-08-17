package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// proxyLinkVideoContent owns the ModelArk Link content contract without
// changing the native OpenAI Videos implementation below the call site.
func proxyLinkVideoContent(c *gin.Context) bool {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		return false
	}
	userID := c.GetInt("id")
	appID := c.GetInt("token_id")
	var task *model.Task
	var exists bool
	var err error
	if appID > 0 {
		task, exists, err = model.GetVideoTaskForProtocol(userID, appID, taskID, model.TaskClientProtocolModelArkV3, true)
	} else {
		task, exists, err = model.GetTaskForProtocol(userID, taskID, model.TaskClientProtocolModelArkV3, true)
	}
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query Link video task %s: %s", taskID, err.Error()))
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "Failed to query task")
		return true
	}
	if !exists || task == nil {
		if appID > 0 {
			_, foreignExists, lookupErr := model.GetTaskForProtocol(userID, taskID, model.TaskClientProtocolModelArkV3, true)
			if lookupErr != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to check Link video task ownership %s: %s", taskID, lookupErr.Error()))
				modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "Failed to query task")
				return true
			}
			if foreignExists {
				modelArkVideoError(c, http.StatusNotFound, "video_not_found", "Task not found")
				return true
			}
		}
		return false
	}
	if task.ClientDeletedAt != 0 {
		modelArkVideoError(c, http.StatusNotFound, "video_not_found", "Task not found")
		return true
	}
	if task.Status != model.TaskStatusSuccess {
		modelArkVideoError(c, http.StatusBadRequest, "video_not_ready", "Video is not ready")
		return true
	}

	var upstream struct {
		ExpiresAt int64 `json:"expires_at"`
		Content   struct {
			LastFrameURL string `json:"last_frame_url"`
		} `json:"content"`
	}
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &upstream)
	}
	if upstream.ExpiresAt > 0 && upstream.ExpiresAt <= common.GetTimestamp() {
		modelArkVideoError(c, http.StatusGone, "video_content_expired", "Video content has expired")
		return true
	}
	contentPart := strings.TrimSpace(c.Query("part"))
	lastFrameURL := ""
	if contentPart != "" {
		if contentPart != "last_frame" {
			modelArkVideoError(c, http.StatusBadRequest, "invalid_content_part", "Content part is not supported")
			return true
		}
		lastFrameURL = strings.TrimSpace(upstream.Content.LastFrameURL)
		if lastFrameURL == "" {
			modelArkVideoError(c, http.StatusNotFound, "content_not_found", "Requested content is not available")
			return true
		}
	}

	channel, err := videoTaskProviderChannel(task)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get frozen Link video channel for task %s: %s", taskID, err.Error()))
		modelArkVideoError(c, http.StatusBadGateway, "frozen_upstream_unavailable", "Frozen video connection details are unavailable")
		return true
	}
	baseURL := strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL)
	if baseURL == "" {
		modelArkVideoError(c, http.StatusBadGateway, "frozen_upstream_unavailable", "Frozen video connection details are unavailable")
		return true
	}

	proxy := channel.GetSetting().Proxy
	client := service.GetSSRFProtectedHTTPClient()
	if proxy != "" {
		client, err = service.GetHttpClientWithProxy(proxy)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create proxy client for Link video task %s: %s", taskID, err.Error()))
			modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "Failed to create proxy client")
			return true
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "Failed to create proxy request")
		return true
	}

	var videoURL string
	rejectRedirects := false
	channelType := channel.Type
	if frozenType, parseErr := strconv.Atoi(string(task.Platform)); parseErr == nil {
		channelType = frozenType
	}
	switch channelType {
	case constant.ChannelTypeGemini:
		apiKey := task.PrivateData.Key
		if apiKey == "" {
			modelArkVideoError(c, http.StatusBadGateway, "frozen_upstream_unavailable", "Frozen video connection details are unavailable")
			return true
		}
		videoURL, err = getGeminiVideoURL(channel, task, apiKey)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Gemini video URL for Link task %s: %s", taskID, sanitizeVideoProviderError(err, apiKey)))
			modelArkVideoError(c, http.StatusBadGateway, "upstream_unavailable", "Failed to resolve video URL")
			return true
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case constant.ChannelTypeVertexAi:
		videoURL, err = getVertexVideoURL(channel, task)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Vertex video URL for Link task %s: %s", taskID, err.Error()))
			modelArkVideoError(c, http.StatusBadGateway, "upstream_unavailable", "Failed to resolve video URL")
			return true
		}
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		key := task.PrivateData.Key
		if key == "" {
			modelArkVideoError(c, http.StatusBadGateway, "frozen_upstream_unavailable", "Frozen video connection details are unavailable")
			return true
		}
		videoURL = fmt.Sprintf("%s/v1/videos/%s/content", baseURL, task.GetUpstreamTaskID())
		req.Header.Set("Authorization", "Bearer "+key)
	case constant.ChannelTypeSeedanceLink:
		contentURL, key, handled, sourceErr := videoFeicaiContentSource(task)
		if sourceErr != nil {
			modelArkVideoError(c, http.StatusBadGateway, "frozen_upstream_unavailable", "Frozen video connection details are unavailable")
			return true
		}
		if handled {
			videoURL = contentURL
			rejectRedirects = true
			req.Header.Set("Authorization", "Bearer "+key)
		} else {
			videoURL = task.GetResultURL()
		}
	default:
		videoURL = task.GetResultURL()
	}
	if lastFrameURL != "" {
		videoURL = lastFrameURL
		req.Header.Del("Authorization")
	}
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		modelArkVideoError(c, http.StatusBadGateway, "upstream_unavailable", "Failed to fetch video content")
		return true
	}
	if strings.HasPrefix(videoURL, "data:") {
		if err := writeLinkVideoDataURL(c, videoURL); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to decode Link video data URL for task %s: %s", taskID, err.Error()))
			modelArkVideoError(c, http.StatusBadGateway, "upstream_unavailable", "Failed to fetch video content")
		}
		return true
	}

	var validateErr error
	if proxy == "" {
		validateErr = service.ValidateSSRFProtectedFetchURL(videoURL)
	} else {
		fetchSetting := system_setting.GetFetchSetting()
		validateErr = common.ValidateURLWithFetchSetting(videoURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain)
	}
	if validateErr != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Link video URL blocked for task %s: %v", taskID, validateErr))
		modelArkVideoError(c, http.StatusForbidden, "content_url_not_allowed", "Video content URL is not allowed")
		return true
	}
	req.URL, err = url.Parse(videoURL)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "Failed to create proxy request")
		return true
	}
	if requestedRange := strings.TrimSpace(c.GetHeader("Range")); requestedRange != "" {
		req.Header.Set("Range", requestedRange)
	}

	if rejectRedirects {
		noRedirectClient := *client
		noRedirectClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &noRedirectClient
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch content for Link video task %s: %s", taskID, sanitizeVideoProviderError(err, task.PrivateData.Key)))
		modelArkVideoError(c, http.StatusBadGateway, "upstream_unavailable", "Failed to fetch video content")
		return true
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d for Link video task %s", resp.StatusCode, taskID))
		modelArkVideoError(c, http.StatusBadGateway, "upstream_unavailable", fmt.Sprintf("Upstream service returned status %d", resp.StatusCode))
		return true
	}

	copySafeVideoContentHeaders(c.Writer.Header(), resp.Header)
	c.Writer.Header().Set("Cache-Control", "private, no-store")
	c.Writer.Header().Set("Pragma", "no-cache")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream Link video content: %s", err.Error()))
	}
	return true
}

func writeLinkVideoDataURL(c *gin.Context, dataURL string) error {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "data:") || !strings.Contains(parts[0], ";base64") {
		return fmt.Errorf("unsupported data url")
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(parts[0], "data:"), ";base64")
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	videoBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		videoBytes, err = base64.RawStdEncoding.DecodeString(parts[1])
		if err != nil {
			return err
		}
	}
	c.Writer.Header().Set("Content-Type", mimeType)
	c.Writer.Header().Set("Cache-Control", "private, no-store")
	c.Writer.Header().Set("Pragma", "no-cache")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(videoBytes)
	return err
}
