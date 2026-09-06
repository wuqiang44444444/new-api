package asyncimage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"net/url"
)

// HeadlessCreateAndPoll executes the FunCloud create+poll lifecycle without a
// client context（异步图片 worker 专用）。共享 adaptor 的 envelope 解析与
// 错误语义；超时由 ctx 控制。返回可信 Provider task ID 与全部有效结果 URL。
func HeadlessCreateAndPoll(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, requestBody io.Reader, onProviderTaskID func(string)) (string, []string, *types.NewAPIError) {
	if info == nil {
		return "", nil, upstreamError("missing relay info")
	}
	requestURL, err := GetCreateURL(info)
	if err != nil {
		return "", nil, upstreamError("failed to build create request URL")
	}
	createResp, apiErr := headlessPost(ctx, info, headers, requestURL, requestBody)
	if apiErr != nil {
		return "", nil, apiErr
	}
	initial, apiErr := headlessReadEnvelope(ctx, createResp, "create request")
	if apiErr != nil {
		return "", nil, apiErr
	}
	if apiErr := envelopeError(initial); apiErr != nil {
		return "", nil, apiErr
	}
	if strings.EqualFold(initial.Data.Status, "success") {
		return "", initial.Data.Result, nil
	}
	if !strings.EqualFold(initial.Data.Status, "processing") && !strings.EqualFold(initial.Data.Status, "queued") {
		return "", nil, upstreamError("unknown create task status")
	}
	if strings.TrimSpace(initial.Data.TaskID) == "" {
		return "", nil, upstreamError("create response did not include task ID")
	}
	// 评审 S6：取得可信 Provider 任务 ID 立即回调持久化，此后崩溃可按
	// 既有任务恢复查询，不重建。
	if onProviderTaskID != nil {
		onProviderTaskID(initial.Data.TaskID)
	}

	return headlessPollTask(ctx, info, headers, initial.Data.TaskID)
}

// HeadlessPollOnly 恢复查询一个已持久化的 Provider 任务（R7：只查询，
// 绝不重建）；返回终态结果 URL，仍处理中返回空切片与 nil 错误。
func HeadlessPollOnly(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, providerTaskID string) ([]string, *types.NewAPIError) {
	pollURL := strings.TrimRight(info.ChannelBaseUrl, "/") + "/api/v2/open/aigc/" + url.PathEscape(providerTaskID)
	resp, apiErr := headlessGet(ctx, info, headers, pollURL)
	if apiErr != nil {
		return nil, apiErr
	}
	result, apiErr := headlessReadEnvelope(ctx, resp, "polling request")
	if apiErr != nil {
		return nil, apiErr
	}
	if apiErr := envelopeError(result); apiErr != nil {
		return nil, apiErr
	}
	switch strings.ToLower(strings.TrimSpace(result.Data.Status)) {
	case "processing", "queued":
		return nil, nil
	case "success":
		return result.Data.Result, nil
	default:
		return nil, upstreamError("unknown polling task status")
	}
}

func headlessPollTask(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, taskID string) (string, []string, *types.NewAPIError) {
	pollURL := strings.TrimRight(info.ChannelBaseUrl, "/") + "/api/v2/open/aigc/" + url.PathEscape(taskID)
	firstPoll := true
	for {
		if !firstPoll {
			if err := headlessWait(ctx, info); err != nil {
				return taskID, nil, headlessContextError(err)
			}
		}
		firstPoll = false
		pollResp, apiErr := headlessGet(ctx, info, headers, pollURL)
		if apiErr != nil {
			return taskID, nil, apiErr
		}
		result, apiErr := headlessReadEnvelope(ctx, pollResp, "polling request")
		if apiErr != nil {
			return taskID, nil, apiErr
		}
		if apiErr := envelopeError(result); apiErr != nil {
			return taskID, nil, apiErr
		}
		switch strings.ToLower(strings.TrimSpace(result.Data.Status)) {
		case "processing", "queued":
			continue
		case "success":
			return taskID, result.Data.Result, nil
		default:
			return taskID, nil, upstreamError("unknown polling task status")
		}
	}
}

// GetCreateURL exposes the create endpoint for the executor.
func GetCreateURL(info *relaycommon.RelayInfo) (string, error) {
	adaptor := &Adaptor{}
	return adaptor.GetRequestURL(info)
}

func headlessPost(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, requestURL string, body io.Reader) (*http.Response, *types.NewAPIError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, body)
	if err != nil {
		return nil, upstreamError("failed to build create request")
	}
	header := http.Header{}
	adaptor := &Adaptor{}
	if err := adaptor.SetupRequestHeader(nil, &header, info); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	req.Header = header
	for key, value := range headers {
		req.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}
	client, err := headlessClient(info)
	if err != nil {
		return nil, upstreamError("failed to initialize upstream client")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, headlessContextError(err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer service.CloseResponseBodyGracefully(resp)
		return nil, headlessHTTPError(resp, "create request")
	}
	return resp, nil
}

func headlessGet(ctx context.Context, info *relaycommon.RelayInfo, headers map[string]string, pollURL string) (*http.Response, *types.NewAPIError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return nil, upstreamError("failed to build polling request")
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}
	client, err := headlessClient(info)
	if err != nil {
		return nil, upstreamError("failed to initialize polling client")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, headlessContextError(err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer service.CloseResponseBodyGracefully(resp)
		return nil, headlessHTTPError(resp, "polling request")
	}
	return resp, nil
}

func headlessReadEnvelope(ctx context.Context, resp *http.Response, operation string) (apiEnvelope, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	envelope, err := decodeEnvelope(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return apiEnvelope{}, headlessContextError(err)
		}
		return apiEnvelope{}, upstreamError("invalid " + operation + " response")
	}
	return envelope, nil
}

func headlessHTTPError(resp *http.Response, operation string) *types.NewAPIError {
	if resp.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if envelope, err := decodeEnvelope(bytes.NewReader(body)); err == nil {
			if apiErr := envelopeError(envelope); apiErr != nil {
				return apiErr
			}
		}
	}
	return upstreamError(operation + " returned an unexpected HTTP status")
}

func headlessClient(info *relaycommon.RelayInfo) (*http.Client, error) {
	adaptor := &Adaptor{}
	return adaptor.httpClient(info)
}

func headlessWait(ctx context.Context, info *relaycommon.RelayInfo) error {
	delay := 5 * time.Second
	if info != nil && !info.StartTime.IsZero() && time.Since(info.StartTime) < 30*time.Second {
		delay = 3 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func headlessContextError(err error) *types.NewAPIError {
	if errors.Is(err, context.DeadlineExceeded) {
		return timeoutError(err)
	}
	return upstreamError("upstream request failed: " + err.Error())
}
