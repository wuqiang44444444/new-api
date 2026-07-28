package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/relayconvert"
)

type mediaImageTaskPollEnvelope struct {
	TaskID       string                      `json:"task_id"`
	ID           string                      `json:"id"`
	RequestID    string                      `json:"request_id"`
	Status       string                      `json:"status"`
	State        string                      `json:"state"`
	Error        any                         `json:"error"`
	ErrorMessage string                      `json:"error_message"`
	Message      string                      `json:"message"`
	Result       *mediaImageTaskPollResult   `json:"result"`
	Usage        json.RawMessage             `json:"usage"`
	Data         *mediaImageTaskPollEnvelope `json:"data"`
}

type mediaImageTaskPollResult struct {
	PrimaryURL string          `json:"primary_url"`
	URLs       []string        `json:"urls"`
	Usage      json.RawMessage `json:"usage"`
}

func (e *mediaImageTaskPollEnvelope) payload() *mediaImageTaskPollEnvelope {
	if e != nil && e.Data != nil {
		return e.Data
	}
	return e
}

func PollMediaImageTaskOnce(ctx context.Context, publicTaskID string) (*model.Task, error) {
	task, exists, err := model.GetByOnlyTaskId(publicTaskID)
	if err != nil {
		return nil, err
	}
	if !exists || task.Platform != constant.TaskPlatformMediaImage || task.ClientProtocol != model.TaskClientProtocolOpenAIImages {
		return nil, errors.New("media image task was not found")
	}
	if task.Status.IsTerminal() {
		return task, nil
	}
	media := task.PrivateData.MediaImage
	if media == nil {
		return task, errors.New("media image task snapshot is missing")
	}
	if media.RequestedImageCount == 0 || media.RequestedImageCount > dto.MaxImageN {
		return finalizeMediaImageTask(ctx, task, nil, nil, "media image task requested count is invalid")
	}

	queryURL, err := mediaImageTaskQueryURL(task)
	if err != nil {
		return task, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, http.NoBody)
	if err != nil {
		return task, fmt.Errorf("create media image task query: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	switch strings.TrimSpace(media.AuthType) {
	case "":
		request.Header.Set("Authorization", "Bearer "+task.PrivateData.Key)
	case dto.AdvancedCustomAuthTypeNone, dto.AdvancedCustomAuthTypeQuery:
	case dto.AdvancedCustomAuthTypeHeader:
		name := strings.TrimSpace(media.AuthName)
		if name == "" {
			return task, errors.New("media image task header auth name is missing")
		}
		request.Header.Set(name, strings.ReplaceAll(media.AuthValueTemplate, "{api_key}", task.PrivateData.Key))
	default:
		return task, errors.New("media image task auth snapshot is invalid")
	}

	client, err := GetHttpClientWithProxy(media.Proxy)
	if err != nil {
		return task, fmt.Errorf("create media image task proxy client: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return task, fmt.Errorf("query upstream media image task: %w", err)
	}
	if response.Body == nil {
		return task, errors.New("upstream media image task response body is empty")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, mediaImageTaskMaxResponseSize+1))
	if err != nil {
		return task, errors.New("read upstream media image task response")
	}
	if len(body) > mediaImageTaskMaxResponseSize {
		return task, errors.New("upstream media image task response is too large")
	}
	if response.StatusCode != http.StatusOK {
		return task, fmt.Errorf("upstream media image task query returned status %d", response.StatusCode)
	}

	var envelope mediaImageTaskPollEnvelope
	if err := common.Unmarshal(body, &envelope); err != nil {
		return task, errors.New("decode upstream media image task response")
	}
	payload := envelope.payload()
	if payload == nil {
		return task, errors.New("upstream media image task response is empty")
	}
	returnedTaskID := strings.TrimSpace(payload.TaskID)
	if returnedTaskID == "" {
		returnedTaskID = strings.TrimSpace(payload.ID)
	}
	if returnedTaskID != "" && returnedTaskID != task.GetUpstreamTaskID() {
		return task, errors.New("upstream media image task response id does not match")
	}
	requestID := mediaImageTaskRequestID(payload.RequestID)
	if requestID == "" {
		requestID = mediaImageTaskRequestID(envelope.RequestID)
	}
	if requestID == "" {
		for _, name := range []string{common.RequestIdKey, "X-Request-Id", "Request-Id", "X-Trace-Id"} {
			if requestID = mediaImageTaskRequestID(response.Header.Get(name)); requestID != "" {
				break
			}
		}
	}
	media.PollAttempts++
	if requestID != "" {
		media.LastPollRequestID = requestID
	}

	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(payload.State))
	}
	switch status {
	case "queued":
		return updateActiveMediaImageTask(task, model.TaskStatusQueued, "0%")
	case "running", "in_progress", "processing":
		return updateActiveMediaImageTask(task, model.TaskStatusInProgress, "50%")
	case "succeeded", "success", "completed":
		rawUsage := payload.Usage
		if payload.Result != nil && len(payload.Result.Usage) > 0 {
			rawUsage = payload.Result.Usage
		}
		usage, err := decodeMediaImageTaskUsage(rawUsage)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("media image task %s ignored invalid usage: %s", task.TaskID, err.Error()))
			usage = nil
		}
		urls, err := mediaImageTaskResultURLs(payload.Result)
		if err != nil {
			return finalizeMediaImageTask(ctx, task, nil, nil, err.Error())
		}
		if len(urls) > int(media.RequestedImageCount) {
			logger.LogWarn(ctx, fmt.Sprintf(
				"media image task %s returned %d images for requested n=%d; failing closed",
				task.TaskID,
				len(urls),
				media.RequestedImageCount,
			))
			return finalizeMediaImageTask(ctx, task, nil, nil, "upstream media image task returned more images than requested")
		}
		return finalizeMediaImageTask(ctx, task, urls, usage, "")
	case "failed", "failure", "cancelled", "canceled", "expired":
		return finalizeMediaImageTask(ctx, task, nil, nil, sanitizeMediaImageTaskFailure(payload.failureMessage()))
	default:
		return updateActiveMediaImageTask(task, model.TaskStatusUnknown, task.Progress)
	}
}

func decodeMediaImageTaskUsage(raw json.RawMessage) (*dto.Usage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var openAIUsage dto.Usage
	var openAIUsageErr error
	if err := common.Unmarshal(raw, &openAIUsage); err == nil {
		if normalized, err := normalizeMediaImageUsage(&openAIUsage); err == nil {
			return normalized, nil
		} else {
			openAIUsageErr = err
		}
	} else {
		openAIUsageErr = err
	}

	var geminiUsage dto.GeminiUsageMetadata
	if err := common.Unmarshal(raw, &geminiUsage); err == nil && dto.HasGeminiUsageMetadataTokens(&geminiUsage) {
		usage := relayconvert.UsageFromGeminiMetadata(&geminiUsage, 0)
		usage.UsageSemantic = dto.BillingUsageSemanticGemini
		return normalizeMediaImageUsage(usage)
	}
	if openAIUsageErr != nil {
		return nil, openAIUsageErr
	}
	return nil, errors.New("upstream media image usage is empty")
}

func updateActiveMediaImageTask(task *model.Task, status model.TaskStatus, progress string) (*model.Task, error) {
	fromStatus := task.Status
	task.Status = status
	task.Progress = progress
	if status == model.TaskStatusInProgress && task.StartTime == 0 {
		task.StartTime = common.GetTimestamp()
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil {
		return task, err
	}
	if won {
		return task, nil
	}
	current, exists, err := model.GetByOnlyTaskId(task.TaskID)
	if err != nil || !exists {
		return task, err
	}
	return current, nil
}

func finalizeMediaImageTask(ctx context.Context, task *model.Task, urls []string, usage *dto.Usage, failure string) (*model.Task, error) {
	fromStatus := task.Status
	now := common.GetTimestamp()
	task.Progress = "100%"
	task.FinishTime = now
	if failure == "" {
		task.Status = model.TaskStatusSuccess
		task.PrivateData.MediaImage.ResultURLs = append([]string(nil), urls...)
		task.PrivateData.MediaImage.Usage = usage
	} else {
		task.Status = model.TaskStatusFailure
		task.FailReason = sanitizeMediaImageTaskFailure(failure)
		if task.PrivateData.AsyncBilling != nil {
			task.PrivateData.AsyncBilling.Operation = "refund"
			task.PrivateData.AsyncBilling.Reason = task.FailReason
		}
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil {
		return task, err
	}
	if !won {
		current, exists, loadErr := model.GetByOnlyTaskId(task.TaskID)
		if loadErr != nil || !exists {
			return task, loadErr
		}
		return current, nil
	}
	if task.Status == model.TaskStatusSuccess {
		settleMediaImageTask(ctx, task)
	} else if task.Quota != 0 {
		refundTaskWithReconcile(ctx, task, task.FailReason)
	}
	return task, nil
}

func UpdateMediaImageTasks(ctx context.Context, taskM map[string]*model.Task) error {
	var firstErr error
	seen := make(map[string]struct{}, len(taskM))
	for _, task := range taskM {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if task == nil {
			continue
		}
		if _, ok := seen[task.TaskID]; ok {
			continue
		}
		seen[task.TaskID] = struct{}{}
		if _, err := PollMediaImageTaskOnce(ctx, task.TaskID); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("media image task %s polling failed: %s", task.TaskID, err.Error()))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func mediaImageTaskQueryURL(task *model.Task) (string, error) {
	media := task.PrivateData.MediaImage
	base, err := url.Parse(strings.TrimSpace(media.QueryBaseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", errors.New("media image task query base URL is invalid")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", errors.New("media image task query base URL must use http or https")
	}
	template := strings.TrimSpace(media.QueryPathTemplate)
	if template == "" {
		template = mediaImageTaskQueryPath
	}
	if !strings.HasPrefix(template, "/") || strings.HasPrefix(template, "//") || strings.Contains(template, "://") {
		return "", errors.New("media image task query path is invalid")
	}
	path := strings.ReplaceAll(template, "{task_id}", url.PathEscape(task.GetUpstreamTaskID()))
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	if strings.TrimSpace(media.AuthType) == dto.AdvancedCustomAuthTypeQuery {
		if strings.TrimSpace(media.AuthName) == "" {
			return "", errors.New("media image task query auth name is missing")
		}
		query := base.Query()
		query.Set(strings.TrimSpace(media.AuthName), strings.ReplaceAll(media.AuthValueTemplate, "{api_key}", task.PrivateData.Key))
		base.RawQuery = query.Encode()
	}
	return base.String(), nil
}

func (e *mediaImageTaskPollEnvelope) failureMessage() string {
	if e == nil {
		return "upstream media image task failed"
	}
	if message := strings.TrimSpace(e.ErrorMessage); message != "" {
		return message
	}
	if message := strings.TrimSpace(e.Message); message != "" {
		return message
	}
	if object, ok := e.Error.(map[string]any); ok {
		if message, ok := object["message"].(string); ok {
			return message
		}
	}
	if message, ok := e.Error.(string); ok && strings.TrimSpace(message) != "" {
		return message
	}
	return "upstream media image task failed"
}

func mediaImageTaskRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	runes := []rune(value)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return value
}

func sanitizeMediaImageTaskFailure(message string) string {
	message = strings.TrimSpace(message)
	lower := strings.ToLower(message)
	for _, sensitive := range []string{"http://", "https://", "bearer ", "api_key", "api-key", "authorization", "cookie"} {
		if strings.Contains(lower, sensitive) {
			return "upstream media image task failed"
		}
	}
	if message == "" {
		return "upstream media image task failed"
	}
	runes := []rune(message)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return message
}
