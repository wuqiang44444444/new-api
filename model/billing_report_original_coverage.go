package model

import (
	"context"
)

// A source row count cannot detect replacement of a known customer original
// with a missing value. Compare the pinned generations before their pointer
// changes. Existing source and lease checks still govern publication.
func verifyCustomerOriginalCoverage(ctx context.Context, previous, candidate string) error {
	return compareCustomerBillingGenerations(ctx, previous, candidate, true)
}

func verifyBillingReportFunding(ctx context.Context, previous, candidate string) error {
	return compareCustomerBillingGenerations(ctx, previous, candidate, false)
}

func compareCustomerBillingGenerations(ctx context.Context, previous, candidate string, preserveOriginal bool) error {
	db := usageReportMainDB(ctx)
	var allowReplayCorrection bool
	if preserveOriginal {
		var prior, next billingReportGeneration
		if err := db.WithContext(ctx).Select("original_replay_version").Where("id = ?", previous).Take(&prior).Error; err != nil {
			return err
		}
		if err := db.WithContext(ctx).Select("original_replay_version").Where("id = ?", candidate).Take(&next).Error; err != nil {
			return err
		}
		allowReplayCorrection = prior.OriginalReplayVersion < next.OriginalReplayVersion
	}
	columns := []string{"source_id", "customer_charge", "log_type", "user_id", "token_id", "channel_id", "created_at", "customer_model", "fund_group"}
	if preserveOriginal {
		columns = append(columns, "customer")
	}
	var cursor int64
	size := dailyReportReadBatchSize(ctx, 500)
	var pacing reportLogCursor
	for {
		if err := pacing.pace(ctx); err != nil {
			return err
		}
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		var prior []billingReportLine
		var next []billingReportLine
		err := executeDailyReportBatch(ctx, func(ctx context.Context) error {
			batch, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
			defer cancel()
			err := db.WithContext(batch).Select(columns).
				Where("generation = ? AND source_id > ?", previous, cursor).
				Order("source_id").Limit(size).Find(&prior).Error
			if err != nil || len(prior) == 0 {
				return err
			}
			cursor = prior[len(prior)-1].SourceID
			sourceIDs := make([]int64, len(prior))
			for i := range prior {
				sourceIDs[i] = prior[i].SourceID
			}
			return db.WithContext(batch).Select(columns).
				Where("generation = ? AND source_id IN ?", candidate, sourceIDs).
				Order("source_id").Find(&next).Error
		})
		if err != nil || len(prior) == 0 {
			return err
		}
		if len(next) != len(prior) {
			return errDailyReportCoverageDecreased
		}
		for i, old := range prior {
			current := &next[i]
			if current.SourceID != old.SourceID ||
				current.UserID != old.UserID || current.LogType != old.LogType ||
				current.TokenID != old.TokenID || current.ChannelID != old.ChannelID || current.CreatedAt != old.CreatedAt ||
				current.CustomerModel != old.CustomerModel || current.FundGroup != old.FundGroup {
				return errDailyReportCoverageDecreased
			}
			if current.CustomerCharge != old.CustomerCharge {
				return errDailyReportCoverageDecreased
			}
			if !preserveOriginal || old.Customer == "" {
				continue
			}
			detail, err := old.customerDetail()
			if err != nil {
				return err
			}
			if detail.Export.RecordedOriginal != "" && current.Customer != old.Customer {
				current, err := current.customerDetail()
				if err != nil {
					return err
				}
				if current.Export.RecordedOriginal != detail.Export.RecordedOriginal {
					// A versioned replay may correct a previously derived
					// amount with new proof, or record a quality reason when
					// historical inputs are incomplete. Settlement evidence
					// remains authoritative.
					wasReplay := detail.Export.OriginalSource == customerOriginalReplayed ||
						detail.Export.OriginalSource == customerOriginalUniqueSettlement
					if !allowReplayCorrection || !wasReplay {
						return errDailyReportCoverageDecreased
					}
				}
			}
		}
		if len(prior) < size {
			return nil
		}
	}
}
