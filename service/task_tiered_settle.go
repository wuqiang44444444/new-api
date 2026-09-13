package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
)

func settleTaskTieredSnapshot(ctx context.Context, task *model.Task, actualTokens int) bool {
	if rejectTaskUsageAsyncBilling(ctx, task) {
		return true
	}
	async := task.PrivateData.AsyncBilling
	if async == nil || async.TieredSnapshot == nil {
		return false
	}
	if task.HasSeedanceBillingFacts() {
		// Re-read accepted facts under the row lock before deriving any funding instruction.
		if err := task.UpdateBilling(); err != nil {
			logger.LogWarn(ctx, "failed to load frozen task billing facts: "+err.Error())
			return true
		}
		async = task.PrivateData.AsyncBilling
		actualTokens = async.ActualTokens
		if async.State != model.TaskBillingStateSettled && async.TargetQuota != nil {
			recalculateTaskQuotaWithReconcile(ctx, task, *async.TargetQuota, async.Reason, async.QuotaClamp)
			return true
		}
	}
	if async.State == model.TaskBillingStateSettled {
		return true
	}
	// 与 prepareTerminalTaskBilling 一致地记录真实 token，供结算/退款日志的 completion_tokens
	// 与提交 UsageFacts 中可能存在的预算投影严格区分：Seedance 已接受事实重读后，
	// actualTokens 只来自持久化的已接受实测（ActualUsageReported 决定可信性），
	// 预算不写入 ActualTokens、不推导出已报告标记、也不落入通用 facts 合并路径。
	async.ActualTokens = actualTokens
	if actualTokens > 0 && !task.HasSeedanceBillingFacts() {
		async.ActualUsageReported = true
	}
	// §5.5 实测依赖规则（与补查、资金目标保护同一实现）：u() 表达式只在读取实测
	// token 时等待；纯冻结条件表达式按冻结事实直接结算；旧 c/_task 表达式保持
	// 通用用量依赖。
	if !async.ActualUsageReported && (!task.HasSeedanceBillingFacts() || seedancebilling.RequiresMeasuredTaskUsage(async.TieredSnapshot)) {
		if awaitSeedanceUsage(ctx, task) {
			return true
		}
		recalculateTaskQuotaWithReconcile(ctx, task, task.Quota, "表达式结算：上游未返回可计费用量，保持预扣额度")
		return true
	}

	result, _, err := ComputeTaskTieredBilling(task)
	if err != nil {
		if result.Clamp != nil {
			async.QuotaClamp = result.Clamp
		}
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, fmt.Errorf("frozen task billing expression failed: %w", err))
		return true
	}
	actualQuota := result.ActualQuotaAfterGroup
	reason := fmt.Sprintf("表达式结算：tokens=%d, tier=%s", actualTokens, result.MatchedTier)
	async.Operation = "settle"
	async.Reason = reason
	async.TargetQuota = &actualQuota
	if err := task.UpdateBilling(); err != nil {
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
		return true
	}
	// A concurrent writer may have installed the target or completed funding.
	async = task.PrivateData.AsyncBilling
	if async.State != model.TaskBillingStateSettled && async.TargetQuota != nil {
		recalculateTaskQuotaWithReconcile(ctx, task, *async.TargetQuota, async.Reason, result.Clamp)
	}
	return true
}
