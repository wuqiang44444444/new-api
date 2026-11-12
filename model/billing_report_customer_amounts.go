package model

import (
	"context"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Historical customer projections store the monetary and usage totals together
// but do not distinguish an original from an estimate. Recover the amount from
// their own immutable lines; the usage and grouping stay on the daily projection.
type billingReportOriginalEvidence struct {
	amount   decimal.Decimal
	debits   int64
	credits  int64
	requests int64
	rows     int64
	missing  int64
}

func (e *billingReportOriginalEvidence) observe(row CustomerExportRow) {
	e.rows++
	e.requests += row.RequestCount
	if row.EventType == "refund" {
		e.credits += max(row.Quota, 0)
	} else {
		e.debits += max(row.Quota, 0)
	}
	if original, err := decimal.NewFromString(row.RecordedOriginal); err == nil {
		e.amount = e.amount.Add(original)
	} else {
		e.missing++
	}
}

func (e billingReportOriginalEvidence) covers(usage BillingReconciliationUsage) bool {
	return e.rows > 0 && e.missing == 0 && e.debits == usage.GrossQuota &&
		e.credits == usage.RefundQuota && e.requests == usage.Requests
}

func (e billingReportOriginalEvidence) apply(price *billingReportPrice) {
	price.Original = decimal.Zero
	price.Complete = e.covers(price.Model.Usage)
	price.Known = price.Complete
	if price.Complete {
		price.Original = e.amount
		return
	}
	price.Model.EstimateReasons = mergeBillingEstimateReasons(price.Model.EstimateReasons, billingOriginalQuotaNotRecorded)
	quality := ensureBillingReconciliationQuality(&price.Model.DataQuality)
	quality.MissingHistoricalPriceRows = max(quality.MissingHistoricalPriceRows, max(1, e.missing))
}

func scanBillingReportCustomerAmounts(ctx context.Context, query *gorm.DB, visit func(billingReportLine, CustomerExportRow)) error {
	var cursor int64
	for {
		var rows []billingReportLine
		if err := query.WithContext(ctx).Select("source_id, user_id, token_id, channel_id, customer").
			Where("source_id > ?", cursor).Order("source_id ASC").Limit(500).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			detail, err := row.customerDetail()
			if err != nil {
				return err
			}
			if detail.Export.CountsTowardBill {
				visit(row, detail.Export)
			}
			cursor = row.SourceID
		}
		if len(rows) < 500 {
			return nil
		}
	}
}

// The legacy line carries the terms but not a contract ID. Match on the shared
// frozen fields; when multiple contract identities match, leave their amounts
// unknown instead of allocating one total twice.
func billingReportAmountCombinationKey(c BillingDiscountCombination) billingDiscountCombinationKey {
	key := billingDiscountKeyFromProjection(c)
	key.contractId, key.contractIdKnown, key.contractVersion = 0, false, 0
	return key
}

func (view *billingReportCustomerDimension) recoverOriginals(ctx context.Context, query *gorm.DB, dimension string) error {
	models := make(map[billingCustomerModelKey]*billingReportOriginalEvidence)
	combinations := make(map[billingDiscountCombinationKey]*billingReportOriginalEvidence)
	for _, leaf := range view.Leaves {
		model := leaf.Price.Model
		models[billingCustomerModelKey{leaf.GroupID, model.ModelName, model.BillingMode}] = new(billingReportOriginalEvidence)
	}
	for _, item := range view.Combinations {
		if item.Combination.Other {
			continue
		}
		key := billingReportAmountCombinationKey(item.Combination)
		if _, exists := combinations[key]; exists {
			combinations[key] = nil
		} else {
			combinations[key] = new(billingReportOriginalEvidence)
		}
	}
	err := scanBillingReportCustomerAmounts(ctx, query, func(line billingReportLine, row CustomerExportRow) {
		groupID := int64(line.TokenID)
		if dimension == "channel" {
			groupID = int64(line.ChannelID)
		}
		if e := models[billingCustomerModelKey{groupID, row.CustomerModel, row.BillingMode}]; e != nil {
			e.observe(row)
		}
		key := billingReportAmountCombinationKey(BillingDiscountCombination{
			GroupId: groupID, ModelName: row.CustomerModel, BillingMode: row.BillingMode,
			GroupName: row.GroupName, GroupRatioSource: row.GroupRatioSource, GroupRatio: row.GroupRatio,
			ContractApplicable: row.ContractApplicable, ContractRatio: row.ContractRatio,
		})
		if e := combinations[key]; e != nil {
			e.observe(row)
		}
	})
	if err != nil {
		return err
	}
	for i := range view.Leaves {
		leaf := &view.Leaves[i]
		model := leaf.Price.Model
		models[billingCustomerModelKey{leaf.GroupID, model.ModelName, model.BillingMode}].apply(&leaf.Price)
	}
	for i := range view.Combinations {
		item := &view.Combinations[i]
		c := &item.Combination
		e := combinations[billingReportAmountCombinationKey(*c)]
		item.Complete = !c.Other && e != nil && e.covers(c.Usage)
		item.Original = decimal.Zero
		if item.Complete {
			item.Original = e.amount
		} else {
			c.EstimateReasons = mergeBillingEstimateReasons(c.EstimateReasons, billingOriginalQuotaNotRecorded)
		}
	}
	return nil
}
