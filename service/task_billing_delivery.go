package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// BuildTaskBillingDeliveryLog projects frozen Task facts; it performs no writes.
func BuildTaskBillingDeliveryLog(task *model.Task, event model.TaskBillingDelivery) (*model.Log, error) {
	copy := *task
	async := model.TaskAsyncBillingContext{}
	if task.PrivateData.AsyncBilling != nil {
		async = *task.PrivateData.AsyncBilling
	}
	copy.PrivateData.AsyncBilling = &async
	async.ActualTokens, async.ActualUsageReported = event.CompletionTokens, event.UsageReported
	async.Operation, async.State = "settle", model.TaskBillingStateSettled
	if event.Event == "create" {
		async.State, async.Operation = model.TaskBillingStatePending, ""
	}
	if event.Event == "complete" {
		async.Operation = ""
	}
	if event.Event == "refund" || event.Event == "customer_refund" {
		async.Operation = "refund"
	}
	if event.Event == "customer_refund" {
		async.Reason = "customer_refund"
	}
	other := taskBillingOther(&copy)
	// A delayed create event must describe the reservation, even when the
	// current Task already has a terminal calculation.
	if event.Event == "create" {
		var initial any
		if bc := task.PrivateData.BillingContext; bc != nil {
			initial = bc.InitialCalculation
		}
		other.SetPublic("billing_calculation", initial)
	}
	if event.Event == "customer_refund" {
		other.SetPublic("refunded_quota", event.BeforeQuota-event.AfterQuota)
		other.SetPublic("net_quota", event.AfterQuota)
	}
	other.SetPublic("task_billing_event", event.Event)
	quota, logType, completion, prompt := event.AfterQuota-event.BeforeQuota, model.LogTypeConsume, event.CompletionTokens, 0
	if event.Event == "create" {
		other.SetPublic("is_task", true)
		if execution := task.PrivateData.Execution; execution != nil {
			other.SetPublic("request_path", execution.RequestPath)
		}
		completion = 0
		if snap := async.TieredSnapshot; snap != nil {
			other.SetPublic("matched_tier", snap.EstimatedTier)
			other.SetPublic("usage_facts", snap.UsageFacts)
			other.SetPublic("expr_b64", base64.StdEncoding.EncodeToString([]byte(snap.ExprString)))
		}
	} else if event.Event == "complete" {
		quota = event.AfterQuota
		appendImageTaskViolationFeeLog(other, &copy, quota)
		other.SetPublic("task_billing_event", "create") // Images log one completed request, without an initial consume row.
		if data := task.PrivateData.ImageTask; data != nil {
			other.SetPublic("image_count", data.ImageCount)
			if usage := data.Usage; usage != nil {
				completion, prompt = usage.CompletionTokens, usage.PromptTokens
				appendImageUsageForLog(other, usage)
				appendImageStatementUsage(other, usage)
			}
		}
	} else if event.Event == "refund" || event.Event == "customer_refund" {
		logType, quota, completion = model.LogTypeRefund, event.BeforeQuota-event.AfterQuota, 0
		other.SetPublic("reason", async.Reason)
	} else {
		other.SetPublic("pre_consumed_quota", event.BeforeQuota)
		other.SetPublic("actual_quota", event.AfterQuota)
		if quota < 0 {
			logType, quota = model.LogTypeRefund, -quota
		}
	}
	attachQuotaSaturationToOther(other, async.QuotaClamp)
	metadata := other.JSONString()
	if metadata == "" {
		return nil, fmt.Errorf("task billing log metadata cannot be encoded")
	}
	return &model.Log{UserId: task.UserId, TokenId: task.PrivateData.TokenId, ChannelId: task.ChannelId, ModelName: taskModelName(task), Group: task.Group,
		Type: logType, Quota: quota, PromptTokens: prompt, CompletionTokens: completion, Other: metadata, Content: async.Reason}, nil
}

func DeliverTaskBillingLogs(ctx context.Context, taskRowID int64, limit int) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	events, err := model.PendingTaskBillingDeliveries(ctx, taskRowID, limit)
	if err != nil {
		logger.LogWarn(ctx, "task billing log delivery scan failed")
		return
	}
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		if err := model.DeliverTaskBillingLog(ctx, event.ID, BuildTaskBillingDeliveryLog); err != nil {
			// The delivery transaction has ended. Persist only the retry schedule
			// after cancellation, with its own bounded DB operation, so a timed-out
			// event cannot monopolize every following scan. Never replay funding.
			if retryErr := model.DeferTaskBillingDelivery(context.WithoutCancel(ctx), event.ID); retryErr != nil {
				logger.LogWarn(ctx, fmt.Sprintf("task billing log delivery %d retry scheduling failed", event.ID))
			}
			logger.LogWarn(ctx, fmt.Sprintf("task billing log delivery %d remains pending", event.ID))
		}
	}
}
