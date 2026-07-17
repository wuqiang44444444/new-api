package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

func settleTaskTieredSnapshot(ctx context.Context, task *model.Task, actualTokens int) bool {
	async := task.PrivateData.AsyncBilling
	if async == nil || async.TieredSnapshot == nil {
		return false
	}
	// 与 prepareTerminalTaskBilling 一致地记录真实 token，供结算/退款日志的 completion_tokens
	// 列回填（recalculateTaskQuotaWithReconcile 读取）。直接调用本函数的路径（如补偿/单测）也生效。
	async.ActualTokens = actualTokens
	if actualTokens <= 0 {
		async.Operation = "settle"
		async.Reason = "表达式结算：上游未返回可计费用量，保持预扣额度"
		target := task.Quota
		async.TargetQuota = &target
		setTaskBillingState(task, model.TaskBillingStateSettled, "")
		if err := task.UpdateBilling(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态回写失败: %s", task.TaskID, err.Error()))
		}
		return true
	}

	requestInput := billingexpr.RequestInput{}
	if async.BillingProbe != nil {
		requestInput = *async.BillingProbe
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(
		async.TieredSnapshot,
		billingexpr.TokenParams{C: float64(actualTokens)},
		requestInput,
	)
	if err != nil {
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, fmt.Errorf("frozen task billing expression failed: %w", err))
		return true
	}

	reason := fmt.Sprintf("表达式结算：tokens=%d, tier=%s", actualTokens, result.MatchedTier)
	async.Operation = "settle"
	async.Reason = reason
	async.TargetQuota = &result.ActualQuotaAfterGroup
	if err := task.UpdateBilling(); err != nil {
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
		return true
	}
	recalculateTaskQuotaWithReconcile(ctx, task, result.ActualQuotaAfterGroup, reason, result.Clamp)
	return true
}
