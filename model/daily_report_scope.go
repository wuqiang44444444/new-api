package model

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
)

var ErrDailyReportDeferred = errors.New("daily report is waiting for a related date")

// Durable task deliveries give both the occurrence day and the task's
// completion day. A related projection can in turn include another refund.
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
	set := map[DailyReportScope]bool{{Start: start, Kind: DailyReportUsage}: true}
	if !usageOnly {
		set[DailyReportScope{Start: start, Kind: DailyReportBilling}] = true
	}
	scopes := make([]DailyReportScope, 0, len(set))
	checked := make(map[DailyReportScope]bool)
	for {
		ranges := db.Session(&gorm.Session{NewDB: true})
		visited := 0
		for scope := range set {
			if checked[scope] {
				continue
			}
			checked[scope] = true
			retrying := scope.Kind == DailyReportUsage && retryUsage[scope.Start] ||
				scope.Kind == DailyReportBilling && retryBilling[scope.Start]
			if retrying && scope.Start < start {
				issues, err := dailyReportCoverageIssues(ctx, scope.Kind, scope.Start, scope.Start+billingReportDaySeconds)
				if err != nil {
					return nil, err
				}
				if issues[scope.Start] {
					// A stronger generation is still published and marked for review.
					continue
				}
			}
			if scope.Start >= today || scope.Start == today-billingReportDaySeconds && now < today+dailyReportCloseDelay || retrying {
				return nil, ErrDailyReportDeferred
			}
			scopes = append(scopes, scope)
			visited++
			if scope.Kind == DailyReportBilling {
				ranges = ranges.Or("delivery.created_at >= ? AND delivery.created_at < ?", scope.Start, scope.Start+billingReportDaySeconds)
			} else {
				ranges = ranges.Or("task.finish_time >= ? AND task.finish_time < ?", scope.Start, scope.Start+billingReportDaySeconds)
			}
		}
		if visited == 0 {
			break
		}
		if err := usageReportBeforeBatch(ctx); err != nil {
			return nil, err
		}
		var events []struct {
			CreatedAt  int64
			FinishTime int64
		}
		err := db.Table("task_billing_deliveries AS delivery").
			Select("delivery.created_at, task.finish_time").
			Joins("JOIN tasks AS task ON task.id = delivery.task_row_id").
			Joins("LEFT JOIN billing_report_heads AS bill ON delivery.created_at >= bill.start AND delivery.created_at < bill.start + 86400").
			Joins("LEFT JOIN usage_report_days AS usage_day ON task.finish_time >= usage_day.start AND task.finish_time < usage_day.start + 86400").
			Where("delivery.delivered_at > 0 AND delivery.suppress_log = ? AND delivery.after_quota < delivery.before_quota AND task.finish_time > 0", false).
			Where(ranges).
			Where("bill.start IS NULL OR delivery.delivered_at > bill.as_of OR usage_day.start IS NULL OR delivery.delivered_at > usage_day.as_of").
			Group("delivery.created_at, task.finish_time").Find(&events).Error
		if err != nil {
			return nil, err
		}
		var related []DailyReportScope
		for _, event := range events {
			at := time.Unix(event.CreatedAt, 0).In(UsageAnalyticsZone())
			occurrence := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location()).Unix()
			at = time.Unix(event.FinishTime, 0).In(UsageAnalyticsZone())
			completion := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location()).Unix()
			if occurrence == completion {
				continue
			}
			for _, scope := range []DailyReportScope{{Start: occurrence, Kind: DailyReportBilling}, {Start: completion, Kind: DailyReportUsage}} {
				if !set[scope] {
					set[scope] = true
					related = append(related, scope)
				}
			}
		}
		if len(related) == 0 {
			break
		}
		// A queued correction for a related day can share this publication.
		// A separately deferred projection keeps its request for a later run.
		revisions, err := DailyReportRebuildRevisions(ctx, related)
		if err != nil {
			return nil, err
		}
		for day, revision := range revisions {
			if revision > 0 && !retryBilling[day] && !retryUsage[day] {
				set[DailyReportScope{Start: day, Kind: DailyReportBilling}] = true
				set[DailyReportScope{Start: day, Kind: DailyReportUsage}] = true
			}
		}
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Start == scopes[j].Start {
			return scopes[i].Kind < scopes[j].Kind
		}
		return scopes[i].Start < scopes[j].Start
	})
	return scopes, nil
}
