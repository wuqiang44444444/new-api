package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const taskCreateAttemptReconcileLimit = 100

func ReconcileTaskCreateAttempts(ctx context.Context) int {
	now := time.Now().Unix()
	attempts := model.GetTaskCreateAttemptsDue(now, taskCreateAttemptReconcileLimit)
	processed := 0
	for _, attempt := range attempts {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		switch attempt.Status {
		case model.TaskCreateAttemptUpstreamSucceeded:
			if _, err := model.RecoverTaskCreateAttempt(attempt.ID); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("recover task create attempt %s failed: %v", attempt.AttemptID, err))
				scheduleTaskCreateAttemptRetry(ctx, attempt, now)
				continue
			}
			processed++
		case model.TaskCreateAttemptSending:
			if err := model.MarkTaskCreateAttemptUnknown(attempt.ID, attempt.UpstreamRequestID); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("mark stale task create attempt %s unknown failed: %v", attempt.AttemptID, err))
				continue
			}
			processed++
		case model.TaskCreateAttemptUnknown:
			if attempt.HoldDeadlineAt > now {
				scheduleTaskCreateAttemptRetry(ctx, attempt, now)
				continue
			}
			if _, err := model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptReleasedWithExposure); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("release unresolved task create attempt %s failed: %v", attempt.AttemptID, err))
				scheduleTaskCreateAttemptRetry(ctx, attempt, now)
				continue
			}
			processed++
		}
	}
	return processed
}

func scheduleTaskCreateAttemptRetry(ctx context.Context, attempt *model.TaskCreateAttempt, now int64) {
	if attempt == nil {
		return
	}
	delay := int64(30 << min(attempt.ReconcileAttempts, 6))
	if err := model.ScheduleTaskCreateAttemptReconcile(attempt.ID, attempt.Status, now+delay); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("schedule task create attempt %s retry failed: %v", attempt.AttemptID, err))
	}
}
