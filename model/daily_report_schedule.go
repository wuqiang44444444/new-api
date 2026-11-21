package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

func NextDailyReportDay(ctx context.Context, today, now int64) (start int64, requestedAt int64, due bool, usageOnly bool, err error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	var usage []usageReportDay
	var billing []billingReportHead
	if err := db.Where("start BETWEEN ? AND ?", today-90*86400, today-86400).Find(&usage).Error; err != nil {
		return 0, 0, false, false, err
	}
	if err := db.Where("start BETWEEN ? AND ?", today-90*86400, today-86400).Find(&billing).Error; err != nil {
		return 0, 0, false, false, err
	}
	retry, err := dailyReportRetryDates(ctx, "", now)
	if err != nil {
		return 0, 0, false, false, err
	}

	usageAsOf := make(map[int64]int64, len(usage))
	for _, head := range usage {
		usageAsOf[head.Start] = head.AsOf
	}
	published := make(map[int64]int64, len(billing))
	for _, head := range billing {
		published[head.Start] = min(usageAsOf[head.Start], head.AsOf)
	}
	yesterday, window := today-86400, today+dailyReportCloseDelay
	if now >= window && !retry[yesterday] && published[yesterday] < window {
		var queued billingReportRebuild
		err := db.Where("start = ?", yesterday).Limit(1).Find(&queued).Error
		return yesterday, queued.RequestedAt, true, false, err
	}
	var request billingReportRebuild
	runnable := db.Where("start < ?", today).Where("NOT EXISTS (SELECT 1 FROM daily_report_retries retry WHERE retry.start = billing_report_rebuilds.start AND "+currentDailyReportRetry+" AND retry.retry_at > ?)", usageReportMeterVersion, CustomerOriginalReplayVersion, now)
	if now < window {
		runnable = runnable.Where("start <> ?", yesterday)
	}
	runnable = runnable.Order("requested_at ASC").Session(&gorm.Session{})
	err = runnable.Where("COALESCE(original_replay_version, 0) = 0").Take(&request).Error
	if err == nil {
		return request.Start, request.RequestedAt, true, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, false, false, err
	}

	// An explicitly deferred day remains eligible even after it leaves the
	// rolling discovery window. Respect both projections' cooldowns.
	var retryDay struct {
		Start        int64
		NeedsBilling int
	}
	pending := db.Model(&dailyReportRetry{}).
		Select("start, MAX(CASE WHEN kind = ? THEN 1 ELSE 0 END) AS needs_billing", DailyReportBilling).
		Where("start > 0 AND start < ? AND "+currentDailyReportRetry, today, usageReportMeterVersion, CustomerOriginalReplayVersion)
	if now < window {
		pending = pending.Where("start <> ?", yesterday)
	}
	err = pending.Group("start").Having("MAX(retry_at) <= ?", now).
		Order("MIN(retry_at) ASC, start ASC").Limit(1).Find(&retryDay).Error
	if err != nil {
		return 0, 0, false, false, err
	}
	if retryDay.Start != 0 {
		var queued billingReportRebuild
		err = db.Where("start = ?", retryDay.Start).Limit(1).Find(&queued).Error
		return retryDay.Start, queued.RequestedAt, true, retryDay.NeedsBilling == 0 && queued.RequestedAt == 0, err
	}

	for _, source := range []struct {
		from, join, predicate string
		args                  []any
	}{
		{"task_billing_deliveries delivery", "JOIN billing_report_heads report ON delivery.created_at >= report.start AND delivery.created_at < report.start + 86400", "delivery.delivered_at >= ? AND delivery.delivered_at > report.as_of", []any{now - 2*86400}},
		{"task_billing_deliveries delivery JOIN tasks task ON task.id = delivery.task_row_id", "JOIN usage_report_days report ON task.finish_time >= report.start AND task.finish_time < report.start + 86400", "delivery.delivered_at >= ? AND delivery.delivered_at > report.as_of", []any{now - 2*86400}},
		{"tasks task", "JOIN usage_report_days report ON task.finish_time >= report.start AND task.finish_time < report.start + 86400", "task.video_refund_state = ? AND task.video_refund_requested_at > 0 AND task.video_refund_requested_at < ? AND report.as_of <= task.video_refund_requested_at + ?", []any{"pending", now - overdueBillingRefundSeconds, overdueBillingRefundSeconds}},
		{"tasks task", "JOIN usage_report_days report ON task.finish_time >= report.start AND task.finish_time < report.start + 86400", "task.video_refund_state = ? AND task.video_refund_completed_at >= ? AND task.video_refund_completed_at > report.as_of", []any{"refunded", now - 2*86400}},
	} {
		var found struct{ Start int64 }
		query := db.Table(source.from).Select("report.start").Joins(source.join).Where("report.start < ?", today)
		if now < window {
			query = query.Where("report.start <> ?", yesterday)
		}
		err := query.Where("NOT EXISTS (SELECT 1 FROM daily_report_retries retry WHERE retry.start = report.start AND "+currentDailyReportRetry+" AND retry.retry_at > ?)", usageReportMeterVersion, CustomerOriginalReplayVersion, now).
			Where(source.predicate, source.args...).Limit(1).Take(&found).Error
		if err == nil {
			return found.Start, 0, true, false, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, false, false, err
		}
	}
	var staleUsage usageReportDay
	stale := db.Select("start").Where("start < ?", today)
	if now < window {
		stale = stale.Where("start <> ?", yesterday)
	}
	err = stale.
		Where("generation <> '' AND COALESCE(meter_version, 0) < ?", usageReportMeterVersion).
		Where("NOT EXISTS (SELECT 1 FROM daily_report_retries retry WHERE retry.start = usage_report_days.start AND retry.kind = ? AND retry.release_version = ? AND retry.retry_at > ?)", DailyReportUsage, usageReportMeterVersion, now).
		Order("start DESC").Take(&staleUsage).Error
	if err == nil {
		return staleUsage.Start, 0, true, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, false, false, err
	}
	err = runnable.Where("original_replay_version > 0").Take(&request).Error
	if err == nil {
		return request.Start, request.RequestedAt, true, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, false, false, err
	}
	for day := yesterday - 86400; day >= today-90*86400; day -= 86400 {
		if !retry[day] && published[day] < day+billingReportDaySeconds+dailyReportCloseDelay {
			return day, 0, true, false, nil
		}
	}
	// Keep the existing one-time closed-day reconciliation for sources
	// without an append or delivery cursor, such as late task/batch updates.
	for day := yesterday - 86400; day >= today-7*86400; day -= 86400 {
		if !retry[day] && now-published[day] >= 86400 && published[day] < day+2*86400 {
			return day, 0, true, false, nil
		}
	}
	return 0, 0, false, false, nil
}
