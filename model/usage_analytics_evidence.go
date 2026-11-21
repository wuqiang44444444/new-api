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
	batchSize := dailyReportReadBatchSize(ctx, 200)
	for start := 0; start < len(agg.errorRows); start += batchSize {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		batch := agg.errorRows[start:min(start+batchSize, len(agg.errorRows))]
		queryCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
		outcomes, sentIDs, err := agg.readFinalErrorEvidence(queryCtx, batch)
		cancel()
		if err != nil {
			return err
		}
		if agg.opts.WantUpstream {
			for _, row := range batch {
				if sentIDs[row.id] && row.fact.ChannelId > 0 {
					acc := agg.buildUpstreamStandalone(row, agg.bundle, 1, usageResultFailure, false, BillingReconciliationModeUnknown, 0, false)
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
					agg.mergeUpstream(usageUpstreamKey{day: usageDayIndex(agg.period, row.fact.CreatedAt), channel: row.fact.ChannelId, model: name, fallback: fallback, mode: BillingReconciliationModeUnknown}, acc)
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

type usageFinalErrorOutcome struct {
	UserID    int
	RequestID string
	LastID    int64
	Consumed  int
}

// Both lookups share the batch deadline. Apply the evidence only after both
// have completed so a timeout cannot leave a partial batch result.
func (agg *usageAggregation) readFinalErrorEvidence(ctx context.Context, batch []usageLogRow) (map[string]usageFinalErrorOutcome, map[int64]bool, error) {
	db := usageReportLogDB(ctx).WithContext(ctx)
	requestIDs := make([]string, 0, len(batch))
	logIDs := make([]int64, 0, len(batch))
	for _, row := range batch {
		if row.fact.RequestId != "" {
			requestIDs = append(requestIDs, row.fact.RequestId)
		}
		logIDs = append(logIDs, row.id)
	}
	outcomes := make(map[string]usageFinalErrorOutcome)
	if len(requestIDs) > 0 {
		query := db.Model(&Log{}).Where("request_id IN ? AND type IN ?", requestIDs, []int{LogTypeConsume, LogTypeError})
		if agg.opts.LogUpper != nil || agg.logUpper > 0 {
			query = query.Where("id <= ?", agg.logUpper)
		}
		if agg.opts.UserID > 0 {
			query = query.Where("user_id = ?", agg.opts.UserID)
		}
		var rows []usageFinalErrorOutcome
		if err := query.Select("user_id, request_id, MAX(id) AS last_id, MAX(CASE WHEN type = ? THEN 1 ELSE 0 END) AS consumed",
			LogTypeConsume).Group("user_id, request_id").Scan(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			outcomes[usageTaskIdentity(row.UserID, row.RequestID)] = row
		}
	}
	if !agg.opts.WantUpstream {
		return outcomes, nil, nil
	}
	var sent []int64
	if err := db.Model(&Log{}).Where("id IN ? AND upstream_request_id <> ''", logIDs).Pluck("id", &sent).Error; err != nil {
		return nil, nil, err
	}
	sentIDs := make(map[int64]bool, len(sent))
	for _, id := range sent {
		sentIDs[id] = true
	}
	return outcomes, sentIDs, nil
}
