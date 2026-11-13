package model

import (
	"context"
	"time"
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
	for {
		var prior []billingReportLine
		var next []billingReportLine
		batch, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.WithContext(batch).Select(columns).
			Where("generation = ? AND source_id > ? AND user_id > 0 AND fund_group <> ?", previous, cursor, "").
			Order("source_id").Limit(500).Find(&prior).Error
		if err == nil && len(prior) != 0 {
			cursor = prior[len(prior)-1].SourceID
			err = db.WithContext(batch).Select(columns).
				Where("generation = ? AND source_id BETWEEN ? AND ? AND user_id > 0 AND fund_group <> ?", candidate, prior[0].SourceID, cursor, "").
				Order("source_id").Find(&next).Error
		}
		cancel()
		if err != nil || len(prior) == 0 {
			return err
		}
		for _, old := range prior {
			for len(next) > 0 && next[0].SourceID < old.SourceID {
				next = next[1:]
			}
			if len(next) == 0 || next[0].SourceID != old.SourceID ||
				next[0].UserID != old.UserID || next[0].LogType != old.LogType ||
				next[0].TokenID != old.TokenID || next[0].ChannelID != old.ChannelID || next[0].CreatedAt != old.CreatedAt ||
				next[0].CustomerModel != old.CustomerModel || next[0].FundGroup != old.FundGroup {
				return errDailyReportCoverageDecreased
			}
			if next[0].CustomerCharge != old.CustomerCharge {
				return errDailyReportCoverageDecreased
			}
			if !preserveOriginal {
				continue
			}
			detail, err := old.customerDetail()
			if err != nil {
				return err
			}
			if detail.Export.RecordedOriginal != "" {
				current, err := next[0].customerDetail()
				if err != nil {
					return err
				}
				if current.Export.RecordedOriginal != detail.Export.RecordedOriginal {
					// A versioned replay may correct a previously derived
					// amount with new proof. Missing evidence or changes to
					// an original written at settlement still block release.
					wasReplay := detail.Export.OriginalSource == customerOriginalReplayed ||
						detail.Export.OriginalSource == customerOriginalUniqueSettlement
					if !allowReplayCorrection || !wasReplay || current.Export.RecordedOriginal == "" {
						return errDailyReportCoverageDecreased
					}
				}
			}
		}
		if len(prior) < 500 {
			return nil
		}
	}
}
