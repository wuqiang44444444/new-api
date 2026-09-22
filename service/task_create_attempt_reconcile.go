package service

import (
	"context"
	"errors"
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
		case model.TaskCreateAttemptPrepared:
			rejected, err := model.RejectPreparedTaskCreateAttempt(attempt.ID)
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("reject stale prepared task create attempt %s failed: %v", attempt.AttemptID, err))
				continue
			}
			if rejected {
				processed++
			}
		case model.TaskCreateAttemptUpstreamSucceeded:
			// 资金保障期限优先于创建恢复：超期记录转入保障退款，不能抢先
			// 补建收费 Task 绕开期限（方案 §4.1/§6 验收项 17）。
			if model.TaskCreateAttemptFundsDeadlineDue(attempt, now) {
				if _, err := model.ReleaseTaskCreateAttemptHoldWarranty(
					attempt.ID, model.TaskCreateAttemptReleaseWarrantyDeadline, 0); err != nil {
					logger.LogWarn(ctx, fmt.Sprintf("warranty refund for task create attempt %s failed: %v", attempt.AttemptID, err))
					if retryErr := model.MarkTaskCreateAttemptRefundRetry(attempt.ID, now, err.Error()); retryErr != nil {
						logger.LogWarn(ctx, fmt.Sprintf("schedule warranty refund retry for task create attempt %s failed: %v", attempt.AttemptID, retryErr))
					}
					continue
				}
				processed++
				continue
			}
			if _, err := model.RecoverTaskCreateAttempt(attempt.ID); err != nil {
				if errors.Is(err, model.ErrTaskCreateAttemptMovedToWarrantyRefund) {
					// 竞争中已被其他入口释放，本记录资金已收尾。
					processed++
					continue
				}
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
			if err := model.StopTaskCreateAttemptReconcile(attempt.ID, model.TaskCreateAttemptUnknown); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("stop automatic reconciliation for unresolved task create attempt %s failed: %v", attempt.AttemptID, err))
				continue
			}
			logger.LogWarn(ctx, fmt.Sprintf("task create attempt %s remains unknown with its customer hold intact; technical verification is required", attempt.AttemptID))
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
