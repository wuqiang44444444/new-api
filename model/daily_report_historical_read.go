package model

import (
	"context"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// A historical usage snapshot can outlive the sources needed to deduplicate
// calls. Refund notices can still be reconciled by their exact task identity.
func reconcileHistoricalUsageRefunds(ctx context.Context, period UsageAnalyticsPeriod, userID int, heads []usageReportDay, report PublishedUsageReport) ([]PublishedUsagePendingRefund, error) {
	days := make(map[int64]bool)
	for _, head := range heads {
		if head.HistoricalSource != "" {
			days[head.Start] = true
		}
	}
	for _, day := range period.Days {
		if slices.Contains(report.SourceUnavailableDays, day.Date) {
			days[day.Start] = true
		}
	}
	if len(days) == 0 {
		return report.PendingRefunds, nil
	}
	type refundFact struct {
		TaskID                 string
		FinishTime             int64
		VideoRefundState       string
		VideoRefundRequestedAt int64
		VideoRefundCompletedAt int64
	}
	var current []refundFact
	err := DB.WithContext(ctx).Model(&Task{}).Select("task_id, finish_time, video_refund_state, video_refund_requested_at, video_refund_completed_at").
		Where("user_id = ? AND finish_time >= ? AND finish_time < ? AND video_refund_requested_at > 0",
			userID, period.StartTimestamp, period.EndTimestamp).Find(&current).Error
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(current))
	notices := make([]PublishedUsagePendingRefund, 0, len(report.PendingRefunds))
	overdueBefore := common.GetTimestamp() - overdueBillingRefundSeconds
	for _, fact := range current {
		day := period.StartTimestamp + (fact.FinishTime-period.StartTimestamp)/billingReportDaySeconds*billingReportDaySeconds
		if !days[day] || fact.TaskID == "" || known[fact.TaskID] {
			continue
		}
		known[fact.TaskID] = true
		if fact.VideoRefundState == "pending" && fact.VideoRefundCompletedAt == 0 && fact.VideoRefundRequestedAt < overdueBefore {
			notices = append(notices, PublishedUsagePendingRefund{TaskID: fact.TaskID, RequestedAt: fact.VideoRefundRequestedAt})
		}
	}
	for _, notice := range report.PendingRefunds {
		if !known[notice.TaskID] {
			notices = append(notices, notice)
		}
	}
	slices.SortFunc(notices, func(a, b PublishedUsagePendingRefund) int { return strings.Compare(a.TaskID, b.TaskID) })
	return notices, nil
}
