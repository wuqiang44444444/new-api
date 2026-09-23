package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Batch execution ends at the provider's terminal timestamp. Settlement and
// file collection may finish later and cannot move requests to a later day.
func (agg *usageAggregation) applyBatchJobs(ctx context.Context) error {
	query := DB.WithContext(ctx).Model(&BatchJob{}).Where(`
 (upstream_status = 'completed' AND completed_at >= ? AND completed_at < ?) OR
 (upstream_status = 'failed' AND failed_at >= ? AND failed_at < ?) OR
 (upstream_status = 'cancelled' AND cancelled_at >= ? AND cancelled_at < ?) OR
 (upstream_status = 'expired' AND expires_at >= ? AND expires_at < ?)`,
		agg.period.StartTimestamp, agg.period.EndTimestamp, agg.period.StartTimestamp, agg.period.EndTimestamp,
		agg.period.StartTimestamp, agg.period.EndTimestamp, agg.period.StartTimestamp, agg.period.EndTimestamp)
	if agg.opts.UserID > 0 {
		query = query.Where("user_id = ?", agg.opts.UserID)
	}
	cursor := ""
	for {
		var jobs []BatchJob
		if err := query.Session(&gorm.Session{}).Where("id > ?", cursor).Order("id").Limit(100).Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		for _, job := range jobs {
			finish := job.CompletedAt
			switch job.UpstreamStatus {
			case "failed":
				finish = job.FailedAt
			case "cancelled":
				finish = job.CancelledAt
			case "expired":
				finish = job.ExpiresAt
			}
			day := usageDayIndex(agg.period, finish)
			total := max(job.CountTotal, job.LineCount)
			success, failed, cancelled := job.CountCompleted, job.CountFailed, job.CountCancelled+job.CountExpired
			if success+failed+cancelled > total {
				return errors.New("batch terminal counters exceed request count")
			}
			base := usageMetricsAcc{}
			base.addCalls(usageResultSuccess, success)
			base.addCalls(usageResultFailure, failed)
			base.addCalls(usageResultCancelled, cancelled)
			base.addCalls(usageResultOther, total-success-failed-cancelled)
			base.m.InputTokens = job.UsageInput
			base.m.OutputTokens = job.UsageOutput
			base.m.CacheReadTokens = job.UsageCached
			base.m.RowsMissingTokens = total - success
			if job.DeliveryState != BatchDeliveryReady {
				base.m.RowsMissingTokens = total
			}
			if job.SettleState != BatchSettleSettled {
				base.m.RowsMoneyPending = total
				base.originalIncomplete = true
			}
			var logs []Log
			if err := LOG_DB.WithContext(ctx).Where("request_id = ? AND user_id = ? AND type IN ?", job.Id, job.UserId, []int{LogTypeConsume, LogTypeRefund}).Find(&logs).Error; err != nil {
				return err
			}
			customer, upstream := usageMetricsAcc{}, usageMetricsAcc{}
			customer.merge(&base)
			upstream.merge(&base)
			for _, log := range logs {
				row := usageLogRow{fact: upstreamBillingLogFact(log, log.Group)}
				row.fact.CreatedAt = finish
				row.parsed = parseBillingReconciliationLog(row.fact)
				_ = common.UnmarshalJsonStr(log.Other, &row.other)
				if agg.opts.WantCustomer {
					customer.observeCustomerDiscount(row)
				}
				if log.Type == LogTypeConsume {
					customer.m.GrossQuota += int64(log.Quota)
				} else {
					customer.m.RefundQuota += int64(log.Quota)
				}
				if original, reasons, known := usageCustomerOriginalRow(row); known {
					customer.original = customer.original.Add(original)
					customer.originalKnown = true
				} else if len(reasons) > 0 {
					customer.originalIncomplete = true
					customer.m.EstimateReasons = mergeBillingEstimateReasons(customer.m.EstimateReasons, reasons...)
				}
				if agg.opts.WantUpstream {
					if original, reference, reasons, coefficient, known := usageUpstreamRowAmount(row, agg.bundle); known {
						upstream.original = upstream.original.Add(original)
						upstream.reference = upstream.reference.Add(reference)
						upstream.originalKnown = true
						upstream.addCoefficient(coefficient)
					} else if len(reasons) > 0 {
						upstream.originalIncomplete = true
						upstream.m.EstimateReasons = mergeBillingEstimateReasons(upstream.m.EstimateReasons, reasons...)
					}
				}
			}
			if len(logs) == 0 || job.SettleState != BatchSettleSettled {
				customer.m.RowsMissingMoney = total
				customer.originalIncomplete = true
				upstream.originalIncomplete = true
				upstream.m.RowsMissingMoney = total
				customer.m.GrossQuota, customer.m.RefundQuota = 0, 0
				if job.SettleState == BatchSettleSettled && job.TargetQuota != nil {
					customer.m.GrossQuota = int64(*job.TargetQuota)
				}
			}
			upstream.m.GrossQuota, upstream.m.RefundQuota = customer.m.GrossQuota, customer.m.RefundQuota
			agg.mergeCustomer(usageCustomerKey{day: day, token: job.TokenId, model: job.PublicModel}, &customer)
			if agg.opts.WantOverview {
				agg.mergeOverview(usageOverviewKey{user: job.UserId, day: day}, &customer)
			}
			if agg.opts.WantUpstream {
				name := job.Deployment
				if name == "" {
					name = job.PublicModel
				}
				agg.mergeUpstream(usageUpstreamKey{day: day, channel: job.ChannelId, model: name, fallback: job.Deployment == "", mode: "azure_batch"}, &upstream)
			}
		}
		cursor = jobs[len(jobs)-1].Id
	}
}
