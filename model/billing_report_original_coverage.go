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
	var cursor int64
	for {
		var prior []billingReportLine
		var next []billingReportLine
		batch, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.WithContext(batch).Select("source_id", "customer", "customer_charge", "log_type", "user_id").
			Where("generation = ? AND source_id > ? AND user_id > 0 AND fund_group <> ?", previous, cursor, "").
			Order("source_id").Limit(500).Find(&prior).Error
		if err == nil && len(prior) != 0 {
			cursor = prior[len(prior)-1].SourceID
			err = db.WithContext(batch).Select("source_id", "customer", "customer_charge", "log_type", "user_id").
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
				next[0].UserID != old.UserID || next[0].LogType != old.LogType {
				return errDailyReportCoverageDecreased
			}
			if next[0].CustomerCharge != old.CustomerCharge {
				if !preserveOriginal {
					return errDailyReportCoverageDecreased
				}
				continue
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
					return errDailyReportCoverageDecreased
				}
			}
		}
		if len(prior) < 500 {
			return nil
		}
	}
}
