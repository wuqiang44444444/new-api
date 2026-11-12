package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

func resolveCustomerOriginalReplay(ctx context.Context, log billingReconciliationLog, parsed *parsedBillingReconciliationLog) error {
	if err := recoverDeliveredCustomerOriginal(ctx, log, parsed); err != nil {
		return err
	}
	if err := recoverBatchCustomerOriginal(ctx, log, parsed); err != nil {
		return err
	}
	applyCustomerOriginalReplay(log, parsed)
	return nil
}

// recoverDeliveredCustomerOriginal uses the immutable delivery checkpoint
// and the matching frozen Task, never a current model or contract setting.
func recoverDeliveredCustomerOriginal(ctx context.Context, log billingReconciliationLog, parsed *parsedBillingReconciliationLog) error {
	if parsed.unavailable || parsed.fundingAdjustmentFailed || parsed.replayedOriginalQuota != nil ||
		parsed.taskID == "" || log.UserId <= 0 || log.TokenId <= 0 {
		return nil
	}
	prefix, ok := strings.CutPrefix(log.RequestId, "task-billing:")
	if !ok {
		return recoverLegacyTaskStatement(ctx, log, parsed)
	}
	id, event, ok := strings.Cut(prefix, ":")
	rowID, err := strconv.ParseInt(id, 10, 64)
	if !ok || err != nil || rowID <= 0 {
		return nil
	}
	switch event {
	case "create", "complete", "refund", "adjustment", "customer_refund":
	default:
		return nil
	}
	if parsed.recordedOriginalQuota != nil && event != "complete" && event != "adjustment" {
		return nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	db := usageReportMainDB(ctx).WithContext(readCtx)
	var delivery TaskBillingDelivery
	err = db.Where("task_row_id = ? AND event = ? AND created_at = ? AND delivered_at > 0 AND suppress_log = ?",
		rowID, event, log.CreatedAt, false).Take(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	delta := int64(delivery.AfterQuota) - int64(delivery.BeforeQuota)
	if event == "complete" {
		delta = int64(delivery.AfterQuota)
	}
	if log.Type == LogTypeRefund {
		delta = -delta
	} else if log.Type != LogTypeConsume {
		return nil
	}
	if delta < 0 || delta != int64(log.Quota) {
		return nil
	}
	var task Task
	err = db.Where("id = ? AND user_id = ? AND app_id = ? AND channel_id = ? AND task_id = ?",
		rowID, log.UserId, log.TokenId, log.ChannelId, parsed.taskID).Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	model := task.Properties.OriginModelName
	if billing := task.PrivateData.BillingContext; billing != nil && billing.OriginModelName != "" {
		model = billing.OriginModelName
	}
	if task.PrivateData.TokenId != log.TokenId || model != parsed.customerModel ||
		(task.PrivateData.AsyncBilling != nil && task.PrivateData.AsyncBilling.QuotaClamp != nil) {
		return nil
	}
	if original := TaskStatementOriginalQuota(&task, delivery, log.Type); parsed.recordedOriginalQuota == nil && original != nil && *original >= 0 {
		value := int64(*original)
		parsed.replayedOriginalQuota = &value
		parsed.originalReplayReason = ""
	}
	if event == "complete" || event == "adjustment" {
		attachTaskContractSettlement(&task, parsed, event, delivery.BeforeQuota, delivery.AfterQuota, log.Type)
	}
	return nil
}
