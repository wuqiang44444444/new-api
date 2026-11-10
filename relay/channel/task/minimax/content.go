package minimax

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
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
	// Official result URLs may expire independently of the JD cache window.
	if task.IsMiniMaxV2Task() {
		observation, _, _, err := queryTaskObservation(ctx, task, "", false)
		if err != nil {
			return "", err
		}
		if observation.Status != observationStatusSucceeded {
			return "", fmt.Errorf("upstream content query returned no trusted video result")
		}
		return observation.VideoURL, nil
	}
	key := contentCacheKey(task)
	if cached, ok := contentCache.Load(key); ok {
		entry := cached.(*contentCacheEntry)
		if time.Since(entry.fetchedAt) < contentCacheTTL {
			return entry.videoURL, nil
		}
		contentCache.Delete(key)
	}
	observation, _, _, err := queryTaskObservation(ctx, task, "", false)
	if err != nil {
		return "", err
	}
	if observation.Status != observationStatusSucceeded || observation.VideoURL == "" {
		return "", fmt.Errorf("upstream content query returned no trusted video result")
	}
	contentCache.Store(key, &contentCacheEntry{videoURL: observation.VideoURL, fetchedAt: time.Now()})
	return observation.VideoURL, nil
}
