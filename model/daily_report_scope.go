package model

import (
	"context"
	"errors"
	"sort"
	"time"
)

var ErrDailyReportDeferred = errors.New("daily report is waiting for a related date")

// Durable task deliveries give both the occurrence day and the task's
// completion day. Include affected published neighbors in the same switch.
func DailyReportScopes(ctx context.Context, start, today, now int64, usageOnly bool) ([]DailyReportScope, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	retryUsage, err := dailyReportRetryDates(ctx, DailyReportUsage, now)
	if err != nil {
		return nil, err
	}
	retryBilling, err := dailyReportRetryDates(ctx, DailyReportBilling, now)
	if err != nil {
		return nil, err
	}
	var events []struct {
		CreatedAt  int64
		FinishTime int64
	}
	err = db.Table("task_billing_deliveries AS delivery").
		Select("delivery.created_at, task.finish_time").
		Joins("JOIN tasks AS task ON task.id = delivery.task_row_id").
		Joins("LEFT JOIN billing_report_heads AS bill ON delivery.created_at >= bill.start AND delivery.created_at < bill.start + 86400").
		Joins("LEFT JOIN usage_report_days AS usage_day ON task.finish_time >= usage_day.start AND task.finish_time < usage_day.start + 86400").
		Where("delivery.delivered_at > 0 AND delivery.suppress_log = ? AND delivery.after_quota < delivery.before_quota AND task.finish_time > 0", false).
		Where("(delivery.created_at >= ? AND delivery.created_at < ?) OR (task.finish_time >= ? AND task.finish_time < ?)", start, start+86400, start, start+86400).
		Where("(bill.start IS NOT NULL AND delivery.delivered_at > bill.as_of) OR (usage_day.start IS NOT NULL AND delivery.delivered_at > usage_day.as_of)").
		Group("delivery.created_at, task.finish_time").Find(&events).Error
	if err != nil {
		return nil, err
	}
	set := map[DailyReportScope]bool{{Start: start, Kind: DailyReportUsage}: true}
	// A version-only rebuild can publish just the usage report unless it
	// also contains a late delivery that requires a coordinated switch.
	if !usageOnly || len(events) > 0 {
		set[DailyReportScope{Start: start, Kind: DailyReportBilling}] = true
	}
	for _, event := range events {
		local := func(ts int64) int64 {
			value := time.Unix(ts, 0).In(UsageAnalyticsZone())
			return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location()).Unix()
		}
		occurrence, completion := local(event.CreatedAt), local(event.FinishTime)
		if occurrence == completion {
			continue
		}
		set[DailyReportScope{Start: occurrence, Kind: DailyReportBilling}] = true
		set[DailyReportScope{Start: completion, Kind: DailyReportUsage}] = true
	}
	scopes := make([]DailyReportScope, 0, len(set))
	for scope := range set {
		retrying := scope.Kind == DailyReportUsage && retryUsage[scope.Start] ||
			scope.Kind == DailyReportBilling && retryBilling[scope.Start]
		if retrying && scope.Start < start {
			issues, err := dailyReportCoverageIssues(ctx, scope.Kind, scope.Start, scope.Start+86400)
			if err != nil {
				return nil, err
			}
			if issues[scope.Start] {
				// A stronger generation is still published and marked for review.
				continue
			}
		}
		if scope.Start == today-86400 && now < today+3*3600 || retrying {
			return nil, ErrDailyReportDeferred
		}
		scopes = append(scopes, scope)
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Start == scopes[j].Start {
			return scopes[i].Kind < scopes[j].Kind
		}
		return scopes[i].Start < scopes[j].Start
	})
	return scopes, nil
}
