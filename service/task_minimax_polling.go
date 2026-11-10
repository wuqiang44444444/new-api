package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

func taskPollingDeferredByUpstreamSchedule(ctx context.Context, task *model.Task, now time.Time) bool {
	if (!task.IsMiniMaxJDTask() && !task.IsMiniMaxV2Task()) || task.PrivateData.VideoUpstreamNextQueryAt <= now.Unix() {
		return false
	}
	logger.LogDebug(ctx, "MiniMax task query deferred task_id=%s next_query_at=%d", task.TaskID, task.PrivateData.VideoUpstreamNextQueryAt)
	return true
}

// MiniMaxTemporaryQueryError reports an unavailable MiniMax status observation.
// StatusCode is zero for a transport error.
type MiniMaxTemporaryQueryError struct {
	StatusCode int
	RetryAfter string
}

func (*MiniMaxTemporaryQueryError) Error() string {
	return "temporary MiniMax task query failure"
}

func handleMiniMaxTemporaryQueryError(ctx context.Context, task *model.Task, err error, now time.Time) (bool, error) {
	var queryErr *MiniMaxTemporaryQueryError
	if (!task.IsMiniMaxJDTask() && !task.IsMiniMaxV2Task()) || !errors.As(err, &queryErr) {
		return false, nil
	}
	var nextAt int64
	if queryErr.StatusCode == http.StatusTooManyRequests {
		retryAfter := strings.TrimSpace(queryErr.RetryAfter)
		// Round up for delay-seconds so storing a Unix second never retries early.
		base := now.Add(time.Second - time.Nanosecond).Unix()
		if seconds, err := strconv.ParseUint(retryAfter, 10, 63); err == nil && seconds <= uint64(math.MaxInt64-base) {
			if seconds > 0 {
				nextAt = base + int64(seconds)
			}
		} else if date, err := http.ParseTime(retryAfter); err == nil {
			nextAt = date.Unix()
		}
	}
	if nextAt > now.Unix() {
		logger.LogWarn(ctx, "MiniMax task query temporarily unavailable task_id=%s http_status=%d retry_not_before=%d", task.TaskID, queryErr.StatusCode, nextAt)
		return true, model.DeferMiniMaxTaskQuery(task.ID, nextAt)
	}
	logger.LogWarn(ctx, "MiniMax task query temporarily unavailable task_id=%s http_status=%d retry=next_poll", task.TaskID, queryErr.StatusCode)
	return true, nil
}
