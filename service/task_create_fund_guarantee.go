package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// Fund guarantee scheduler for video creation holds.
// Contract: docs/80-dev/2026-09-22-视频创建未知结果长期占款问题分析与退款闭环方案.md §4.2/§4.3.
// Single authority deciding the warranty refund policy; the technical
// reconcile loop never decides refunds.

// TaskCreateFundSummary summarizes one fund guarantee pass.
type TaskCreateFundSummary struct {
	ScanFailed    bool  `json:"scan_failed"`
	Debts         int   `json:"debts"`
	Released      int   `json:"released"`
	ReleasedQuota int64 `json:"released_quota"`
	Noops         int   `json:"noops"`
	Failed        int   `json:"failed"`
	Blocked       int   `json:"blocked"`
}

// RunTaskCreateFundGuaranteeOnce runs one pass: release overdue creation holds
// to the customer and keep retrying failed refunds. Failures never cap out:
// every failed item gets fund_retry_at advanced and an alarm log line, so a
// stuck row stays visible instead of silently disappearing.
func RunTaskCreateFundGuaranteeOnce(ctx context.Context) TaskCreateFundSummary {
	summary := TaskCreateFundSummary{}
	now := model.GetDBTimestamp()
	attempts, scanErr := model.GetTaskCreateAttemptFundDebts(now, model.TaskCreateFundScanLimit)
	if scanErr != nil {
		summary.ScanFailed = true
		logger.LogWarn(ctx, "video fund guarantee scan failed")
		return summary
	}
	summary.Debts = len(attempts)
	for _, attempt := range attempts {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		var result *model.TaskAttemptReleaseResult
		var err error
		if attempt.FundTarget == "verified_rejection" {
			result, err = model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptRejected)
		} else {
			result, err = model.ReleaseTaskCreateAttemptHoldWarranty(attempt.ID, model.TaskCreateAttemptReleaseWarrantyDeadline, 0)
		}
		if err != nil {
			if errors.Is(err, model.ErrTaskCreateAttemptFundBlocked) {
				summary.Blocked++
			} else {
				summary.Failed++
			}
			common.SysError("task create fund guarantee failed: attempt_id=" + attempt.AttemptID + " err=" + err.Error())
			if retryErr := model.MarkTaskCreateAttemptRefundRetry(
				attempt.ID, model.GetDBTimestamp(), err.Error()); retryErr != nil {
				common.SysError("task create fund guarantee retry marker failed: attempt_id=" + attempt.AttemptID + " err=" + retryErr.Error())
			}
			logger.LogWarn(ctx, "task create fund guarantee retry scheduled: attempt_id="+attempt.AttemptID+" reason="+err.Error())
			continue
		}
		if result == nil || result.ReleasedQuota == 0 {
			// Nothing moved (a zero hold or idempotent rerun);
			// the row is terminal for fund work either way.
			summary.Noops++
			continue
		}
		summary.Released++
		summary.ReleasedQuota += int64(result.ReleasedQuota)
	}
	return summary
}
