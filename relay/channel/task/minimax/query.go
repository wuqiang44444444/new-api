package minimax

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

const maxTaskObservationBytes = 4 << 20

// queryTaskObservation uses only the frozen connection and artifact. The
// delivery worker reads the observation without applying it to task state.
func queryTaskObservation(ctx context.Context, task *model.Task, key string, captureEvidence bool) (*normalizedObservation, []byte, *http.Response, error) {
	if task == nil || task.Platform != Platform {
		return nil, nil, nil, fmt.Errorf("MiniMax Link task is required")
	}
	frozen := task.PrivateData
	_, queryPath, registered := constant.MiniMaxVideoPaths(string(frozen.VideoUpstreamProtocol))
	if !registered {
		return nil, nil, nil, &relaycommon.UpstreamContractViolation{Reason: "unregistered frozen MiniMax video protocol"}
	}
	taskID := frozen.UpstreamTaskID
	pathTemplate := frozen.VideoUpstreamQueryPathTemplate
	official := task.IsMiniMaxV2Task()
	if !official {
		taskID = task.GetUpstreamTaskID()
		if pathTemplate == "" {
			pathTemplate = queryPath
		}
	}
	if frozen.Key != "" || official {
		key = frozen.Key
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(key) == "" ||
		strings.TrimSpace(frozen.VideoUpstreamQueryBaseURL) == "" || (official && pathTemplate != queryPath) {
		return nil, nil, nil, &relaycommon.UpstreamContractViolation{Reason: "incomplete frozen MiniMax query connection"}
	}
	plugin, err := frozenObservationPlugin(ctx, task)
	if err != nil {
		return nil, nil, nil, err
	}
	path := strings.Replace(pathTemplate, "{task_id}", url.PathEscape(taskID), 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinProviderURL(frozen.VideoUpstreamQueryBaseURL, path), nil)
	if err != nil {
		return nil, nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid frozen MiniMax query connection"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(frozen.VideoUpstreamProxy)
	if err != nil {
		return nil, nil, nil, relaycommon.ErrUpstreamObservationUnavailable
	}
	if official {
		direct := *client
		direct.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &direct
	}
	resp, err := client.Do(req)
	if captureEvidence {
		service.AttachRawTaskPollingEvidence(task, req, resp, err)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
		return nil, nil, nil, &service.MiniMaxTemporaryQueryError{}
	}
	defer resp.Body.Close()
	logger.LogDebug(ctx, "MiniMax task query response task_id=%s http_status=%d", task.TaskID, resp.StatusCode)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= http.StatusInternalServerError {
			return nil, nil, nil, &service.MiniMaxTemporaryQueryError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
		}
		return nil, nil, nil, &relaycommon.UpstreamContractViolation{Reason: fmt.Sprintf("unverified MiniMax query HTTP status %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTaskObservationBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
		return nil, nil, nil, &service.MiniMaxTemporaryQueryError{}
	}
	if len(body) > maxTaskObservationBytes {
		return nil, nil, nil, &relaycommon.UpstreamContractViolation{Reason: "MiniMax observation exceeds the response limit"}
	}
	observation, normalized, err := normalizeTaskObservation(ctx, plugin, task, body, taskID)
	return observation, normalized, resp, err
}
