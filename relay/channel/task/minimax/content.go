package minimax

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// contentCacheEntry is a short-lived in-memory merged-query cache. It binds
// to the task and its frozen execution identity, is rebuildable, and never
// becomes the authority for task success or funds: a missing entry only
// triggers one on-demand JD query through the frozen connection.
type contentCacheEntry struct {
	videoURL  string
	fetchedAt time.Time
}

const contentCacheTTL = 10 * time.Minute

var contentCache sync.Map

func contentCacheKey(task *model.Task) string {
	snapshot := task.PrivateData.Execution.TaskPlugin
	digest := sha256.Sum256([]byte(strings.Join([]string{
		task.TaskID,
		task.GetUpstreamTaskID(),
		task.PrivateData.VideoUpstreamQueryBaseURL,
		task.PrivateData.Key,
		snapshot.Key,
		snapshot.Version,
	}, "\n")))
	return hex.EncodeToString(digest[:])
}

// ResolveContentURL returns a fresh CDN URL for a successful task. A cached
// verified address is reused within its TTL; otherwise exactly one on-demand
// JD GET runs through the frozen connection and the frozen artifact version.
// This path is never blocked by the background polling cadence and is never a
// provider creation retry.
func ResolveContentURL(ctx context.Context, task *model.Task) (string, error) {
	if task == nil || task.Status != model.TaskStatusSuccess {
		return "", fmt.Errorf("video content is not ready")
	}
	if task.PrivateData.Execution == nil || task.PrivateData.Execution.TaskPlugin == nil {
		return "", fmt.Errorf("frozen execution identity is unavailable")
	}
	key := contentCacheKey(task)
	if cached, ok := contentCache.Load(key); ok {
		entry := cached.(*contentCacheEntry)
		if time.Since(entry.fetchedAt) < contentCacheTTL {
			return entry.videoURL, nil
		}
		contentCache.Delete(key)
	}
	taskID := task.GetUpstreamTaskID()
	if strings.TrimSpace(taskID) == "" {
		return "", fmt.Errorf("frozen upstream task id is unavailable")
	}
	baseURL := task.PrivateData.VideoUpstreamQueryBaseURL
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("frozen upstream query base URL is unavailable")
	}
	path := strings.Replace(jdCloudQueryPathTemplate, "{task_id}", taskID, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinProviderURL(baseURL, path), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+task.PrivateData.Key)
	client, err := service.GetHttpClientWithProxy(task.PrivateData.VideoUpstreamProxy)
	if err != nil {
		return "", fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("upstream content query returned HTTP %d", resp.StatusCode)
	}
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("read upstream content response: %w", err)
	}
	observation, _, err := normalizeTaskObservation(ctx, task, rawBody, taskID)
	if err != nil {
		return "", err
	}
	if observation.Status != observationStatusSucceeded || observation.VideoURL == "" {
		return "", fmt.Errorf("upstream content query returned no trusted video result")
	}
	contentCache.Store(key, &contentCacheEntry{videoURL: observation.VideoURL, fetchedAt: time.Now()})
	return observation.VideoURL, nil
}
