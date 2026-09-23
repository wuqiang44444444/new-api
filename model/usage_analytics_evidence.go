package model

import (
	"context"
	"fmt"
)

func usageTaskIdentity(userID int, taskID string) string {
	return fmt.Sprintf("%d:%s", userID, taskID)
}

func usageDiscountSource(record ProviderChannelBillingDiscount) string {
	if record.Id == 0 {
		return "default"
	}
	return "database"
}

// Error logs describe attempts. Only the last failed attempt represents the
// customer's outcome. A final consume owns the outcome (including stream
// failure/cancellation), so previous error attempts must not count it again.
// Look outside the selected period so a retry across midnight is not counted
// once on each day. The log high-water mark also bounds these lookups.
func (agg *usageAggregation) applyFinalErrors(ctx context.Context) error {
	for start := 0; start < len(agg.errorRows); start += 200 {
		batch := agg.errorRows[start:min(start+200, len(agg.errorRows))]
		ids := make([]string, 0, len(batch))
		for _, row := range batch {
			if row.fact.RequestId != "" {
				ids = append(ids, row.fact.RequestId)
			}
		}
		type outcome struct {
			UserId    int
			RequestId string
			LastID    int64
			Consumed  int
		}
		outcomes := make(map[string]outcome)
		if len(ids) > 0 {
			query := LOG_DB.WithContext(ctx).Model(&Log{}).Where("request_id IN ? AND type IN ?", ids, []int{LogTypeConsume, LogTypeError})
			if agg.logUpper > 0 {
				query = query.Where("id <= ?", agg.logUpper)
			}
			if agg.opts.UserID > 0 {
				query = query.Where("user_id = ?", agg.opts.UserID)
			}
			var rows []outcome
			if err := query.Select("user_id, request_id, MAX(id) AS last_id, MAX(CASE WHEN type = ? THEN 1 ELSE 0 END) AS consumed", LogTypeConsume).Group("user_id, request_id").Scan(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				outcomes[usageTaskIdentity(row.UserId, row.RequestId)] = row
			}
		}
		// A recorded provider request ID proves an upstream interaction. Local
		// validation/routing errors cannot be promoted to provider failures.
		if agg.opts.WantUpstream {
			logIDs := make([]int64, 0, len(batch))
			for _, row := range batch {
				logIDs = append(logIDs, row.id)
			}
			var sent []Log
			if err := LOG_DB.WithContext(ctx).Select("id").Where("id IN ? AND upstream_request_id <> ''", logIDs).Find(&sent).Error; err != nil {
				return err
			}
			sentIDs := make(map[int64]bool, len(sent))
			for _, row := range sent {
				sentIDs[int64(row.Id)] = true
			}
			for _, row := range batch {
				if sentIDs[row.id] && row.fact.ChannelId > 0 {
					acc := agg.buildUpstreamStandalone(row, agg.bundle, 1, usageResultFailure, false)
					acc.m.RowsMissingMoney = 1
					acc.originalIncomplete = true
					name := row.parsed.providerModel
					fallback := name == ""
					if fallback {
						name = row.fact.ModelName
					}
					if name == "" {
						name = usageAnalyticsUnknownModel
					}
					agg.mergeUpstream(usageUpstreamKey{day: usageDayIndex(agg.period, row.fact.CreatedAt), channel: row.fact.ChannelId, model: name, fallback: fallback, mode: row.parsed.billingMode}, acc)
				}
			}
		}
		for _, row := range batch {
			if row.fact.RequestId != "" {
				final := outcomes[usageTaskIdentity(row.fact.UserId, row.fact.RequestId)]
				if final.Consumed > 0 || final.LastID != row.id {
					continue
				}
			}
			agg.applyStandalone(row, agg.bundle, 1, usageResultFailure, false)
		}
	}
	return nil
}
