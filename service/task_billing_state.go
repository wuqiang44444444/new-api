package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func setTaskBillingState(task *model.Task, state model.TaskBillingState, billingErr string) {
	async := task.PrivateData.AsyncBilling
	if async == nil {
		return
	}
	async.State = state
	async.Error = billingErr
	if state == model.TaskBillingStateFailed || state == model.TaskBillingStateDebt {
		async.Attempts++
		delay := time.Duration(1<<min(async.Attempts, 8)) * time.Second
		async.NextRetryAt = time.Now().Add(delay).Unix()
	} else {
		async.NextRetryAt = 0
	}
}

func persistTaskBillingFailure(ctx context.Context, task *model.Task, state model.TaskBillingState, err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	firstFailure := task.PrivateData.AsyncBilling != nil && task.PrivateData.AsyncBilling.Attempts == 0
	setTaskBillingState(task, state, message)
	if updateErr := task.UpdateBilling(); updateErr != nil {
		logger.LogError(ctx, fmt.Sprintf("任务 %s 计费状态写入失败: %s", task.TaskID, updateErr.Error()))
	}
	logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费进入 %s: %s", task.TaskID, state, message))
	if firstFailure {
		other := taskBillingOther(task)
		other["task_id"] = task.TaskID
		other["admin_info"] = map[string]any{
			"task_billing_state": state,
			"task_billing_error": message,
		}
		if task.PrivateData.AsyncBilling.QuotaClamp != nil {
			attachQuotaSaturationToOther(other, task.PrivateData.AsyncBilling.QuotaClamp)
		}
		model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
			UserId: task.UserId, LogType: model.LogTypeConsume, Content: "异步任务计费异常",
			ChannelId: task.ChannelId, ModelName: taskModelName(task), Quota: 0,
			TokenId: task.PrivateData.TokenId, Group: task.Group, Other: other,
			NodeName: task.PrivateData.NodeName,
		})
	}
}

func refundTaskWithReconcile(ctx context.Context, task *model.Task, reason string) {
	async := task.PrivateData.AsyncBilling
	if async == nil {
		RefundTaskQuota(ctx, task, reason)
		return
	}
	if async.State == model.TaskBillingStateSettled {
		return
	}
	quota := task.Quota
	applied, _, err := model.ApplyTaskBillingTarget(task, 0)
	if err != nil {
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
		return
	}
	if !applied {
		return
	}
	if quota == 0 {
		return
	}

	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: task.UserId, LogType: model.LogTypeRefund, ChannelId: task.ChannelId,
		ModelName: taskModelName(task), Quota: quota, TokenId: task.PrivateData.TokenId,
		Group: task.Group, Other: other,
	})
}

func recalculateTaskQuotaWithReconcile(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	async := task.PrivateData.AsyncBilling
	if async == nil {
		RecalculateTaskQuota(ctx, task, actualQuota, reason, clamps...)
		return
	}
	if actualQuota < 0 || async.State == model.TaskBillingStateSettled {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota
	if async.TieredSnapshot != nil && quotaDelta > 0 {
		for _, clamp := range clamps {
			if clamp != nil {
				async.QuotaClamp = clamp
				break
			}
		}
		err := fmt.Errorf("actual quota %d exceeds pre-consumed upper bound %d", actualQuota, preConsumedQuota)
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateDebt, err)
		return
	}
	applied, quotaDelta, err := model.ApplyTaskBillingTarget(task, actualQuota)
	if err != nil {
		persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
		return
	}
	if !applied {
		return
	}
	if quotaDelta == 0 {
		return
	}

	logType, logQuota := model.LogTypeRefund, -quotaDelta
	if quotaDelta > 0 {
		logType, logQuota = model.LogTypeConsume, quotaDelta
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: task.UserId, LogType: logType, Content: reason, ChannelId: task.ChannelId,
		ModelName: taskModelName(task), Quota: logQuota, TokenId: task.PrivateData.TokenId,
		Group: task.Group, Other: other, NodeName: task.PrivateData.NodeName,
		CompletionTokens: async.ActualTokens,
	})
}

func calculateTaskQuotaByTokens(task *model.Task, totalTokens int) (int, *common.QuotaClamp, string, bool) {
	if totalTokens <= 0 {
		return 0, nil, "", false
	}
	modelRatio, ok, _ := ratio_setting.GetModelRatio(taskModelName(task))
	if !ok || modelRatio <= 0 {
		return 0, nil, "", false
	}
	group := task.Group
	if group == "" {
		user, err := model.GetUserById(task.UserId, false)
		if err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return 0, nil, "", false
	}
	groupRatio := ratio_setting.GetGroupRatio(group)
	if special, found := ratio_setting.GetGroupGroupRatio(group, group); found {
		groupRatio = special
	}
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(task.PrivateData.BillingContext); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}
	quota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * groupRatio * otherMultiplier)
	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, groupRatio, otherMultiplier)
	return quota, clamp, reason, true
}

func settleTaskBillingWithState(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, result *relaycommon.TaskInfo) bool {
	async := task.PrivateData.AsyncBilling
	if async == nil {
		return false
	}
	if task.PrivateData.BillingContext != nil && task.PrivateData.BillingContext.PerCallBilling {
		setTaskBillingState(task, model.TaskBillingStateSettled, "")
		if err := task.UpdateBilling(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态回写失败: %s", task.TaskID, err.Error()))
		}
		return true
	}
	actualTokens := result.CompletionTokens
	if actualTokens <= 0 {
		actualTokens = result.TotalTokens
	}
	if settleTaskTieredSnapshot(ctx, task, actualTokens) {
		return true
	}
	if actualQuota := adaptor.AdjustBillingOnComplete(task, result); actualQuota > 0 {
		async.Operation = "settle"
		async.Reason = "adaptor计费调整"
		async.TargetQuota = &actualQuota
		if err := task.UpdateBilling(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态回写失败: %s", task.TaskID, err.Error()))
		}
		recalculateTaskQuotaWithReconcile(ctx, task, actualQuota, "adaptor计费调整")
		return true
	}
	if actualQuota, clamp, reason, ok := calculateTaskQuotaByTokens(task, result.TotalTokens); ok {
		recalculateTaskQuotaWithReconcile(ctx, task, actualQuota, reason, clamp)
		return true
	}
	setTaskBillingState(task, model.TaskBillingStateSettled, "")
	if err := task.UpdateBilling(); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态回写失败: %s", task.TaskID, err.Error()))
	}
	return true
}
