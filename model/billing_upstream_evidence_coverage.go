package model

import (
	"context"
	"github.com/shopspring/decimal"
)

// BillingEvidenceCoverage counts distinct scanned log rows, not requests or
// issue occurrences. One row with several evidence gaps contributes only once.
// Task holds, settlements and refunds may be separate rows for one request.
type BillingEvidenceCoverage struct {
	Rows    int64 `json:"rows"`
	GapRows int64 `json:"gap_rows"`
	// Exclusive reasons: missing amount first, then usage, then other facts.
	AmountGapRows int64 `json:"amount_gap_rows"`
	UsageGapRows  int64 `json:"usage_gap_rows"`
	OtherGapRows  int64 `json:"other_gap_rows"`
}

// Coverage uses the detail view's evidence classification and seconds recovery
// so the report can be reproduced from its exported rows. The keyset reader
// visits each log ID once; only a bounded batch of row facts is retained.
type upstreamRecoveredSecondsKey struct {
	task upstreamTaskSecondsKey
	item providerBillingSummaryKey
}

type upstreamEvidenceCoverage struct {
	rows      []UpstreamBillingDetailItem
	keys      []providerBillingSummaryKey
	recovered map[upstreamRecoveredSecondsKey]billingRecoveredSeconds
	onRow     func(UpstreamBillingDetailItem, providerBillingSummaryKey) error
}

type billingRecoveredSeconds struct {
	Value    decimal.Decimal
	SourceID int64
}

func (coverage *upstreamEvidenceCoverage) observe(key providerBillingSummaryKey, log Log, fact billingReconciliationLog, parsed parsedBillingReconciliationLog, includeRoot bool) {
	coverage.rows = append(coverage.rows, projectUpstreamBillingDetail(log, fact, parsed, includeRoot))
	coverage.keys = append(coverage.keys, key)
}

func (coverage *upstreamEvidenceCoverage) flush(ctx context.Context, items map[providerBillingSummaryKey]*ProviderBillingPlatformSummary) error {
	if coverage.onRow != nil {
		if err := resolveUpstreamDetailRequestIDs(ctx, coverage.rows); err != nil {
			return err
		}
	}
	if err := flushRecoveredDetailSeconds(ctx, coverage.rows); err != nil {
		return err
	}
	for i, row := range coverage.rows {
		q := ensureBillingReconciliationQuality(&items[coverage.keys[i]].DataQuality)
		q.RecoveredBillingSecondsRows += row.DataQuality.RecoveredBillingSecondsRows
		q.RefundedTaskHoldRows += row.DataQuality.RefundedTaskHoldRows
		q.SecondsTaskLinkMissingRows += row.DataQuality.SecondsTaskLinkMissingRows
		if row.secondsRecovered {
			item := items[coverage.keys[i]]
			item.Usage.SecondsUnavailableRows--
			q.SecondsUnavailableRows--
			q.SecondsValueMissingRows--
			key := upstreamRecoveredSecondsKey{upstreamTaskSecondsKey{row.secondsLog.UserId, row.secondsLog.ChannelId, row.PlatformTaskId}, coverage.keys[i]}
			if coverage.recovered == nil {
				coverage.recovered = make(map[upstreamRecoveredSecondsKey]billingRecoveredSeconds)
			}
			if _, found := coverage.recovered[key]; row.Seconds != nil && !found {
				value := *row.Seconds
				if item.Usage.Seconds != nil {
					value = value.Add(*item.Usage.Seconds)
				}
				item.Usage.Seconds = &value
				coverage.recovered[key] = billingRecoveredSeconds{*row.Seconds, row.RowId}
			}
		}

		if q.EvidenceCoverage == nil {
			q.EvidenceCoverage = &BillingEvidenceCoverage{}
		}
		q.EvidenceCoverage.Rows++
		if category := upstreamEvidenceCoverageCategory(row); category != "complete" {
			q.EvidenceCoverage.GapRows++
			switch category {
			case "amount_gap":
				q.EvidenceCoverage.AmountGapRows++
			case "usage_gap":
				q.EvidenceCoverage.UsageGapRows++
			default:
				q.EvidenceCoverage.OtherGapRows++
			}
		}
		if coverage.onRow != nil {
			if err := coverage.onRow(row, coverage.keys[i]); err != nil {
				return err
			}
		}
	}
	coverage.rows = coverage.rows[:0]
	coverage.keys = coverage.keys[:0]
	return nil
}
