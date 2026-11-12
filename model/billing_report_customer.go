package model

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

type billingReportPrice struct {
	RecordedAmounts  bool
	Model            BillingReconciliationModelSummary
	Original         decimal.Decimal
	Known            bool
	Complete         bool
	GroupRatio       *float64
	ContractRatio    *float64
	ContractFact     bool
	MultipleContract bool
	Markers          map[string]struct{}
}

func projectBillingReportPrice(acc *billingReconciliationModelAccumulator) billingReportPrice {
	row := billingReportPrice{RecordedAmounts: true, Model: acc.model, Original: acc.originalQuota, Known: acc.originalQuotaKnown,
		Complete: acc.originalQuotaComplete, ContractFact: acc.hasContractDiscountFact,
		MultipleContract: acc.multipleContractDiscounts}
	if len(acc.priceSnapshotMarkers) > 0 {
		row.Markers = make(map[string]struct{}, len(acc.priceSnapshotMarkers))
		for marker := range acc.priceSnapshotMarkers {
			row.Markers[hex.EncodeToString(common.Sha256Raw([]byte(marker)))] = struct{}{}
		}
	}
	if acc.discountSeen {
		row.GroupRatio = &acc.discountRatio
	}
	if acc.contractDiscountSeen {
		row.ContractRatio = &acc.contractDiscountRatio
	}
	return row
}

func (row billingReportPrice) mergeInto(acc *billingReconciliationModelAccumulator) {
	accumulateBillingReconciliationUsage(&acc.model.Usage, row.Model.Usage)
	accumulateBillingReconciliationQuality(&acc.model.DataQuality, row.Model.DataQuality)
	acc.model.EstimateReasons = mergeBillingEstimateReasons(acc.model.EstimateReasons, row.Model.EstimateReasons...)
	acc.model.MultipleDiscounts = acc.model.MultipleDiscounts || row.Model.MultipleDiscounts
	acc.originalQuota = acc.originalQuota.Add(row.Original)
	acc.originalQuotaKnown = acc.originalQuotaKnown || row.Known
	acc.originalQuotaComplete = acc.originalQuotaComplete && row.Complete
	acc.hasContractDiscountFact = acc.hasContractDiscountFact || row.ContractFact
	acc.multipleContractDiscounts = acc.multipleContractDiscounts || row.MultipleContract
	if row.GroupRatio != nil {
		acc.model.MultipleDiscounts = acc.model.MultipleDiscounts || (acc.discountSeen && acc.discountRatio != *row.GroupRatio)
		acc.discountSeen, acc.discountRatio = true, *row.GroupRatio
	}
	if row.ContractRatio != nil {
		acc.multipleContractDiscounts = acc.multipleContractDiscounts || (acc.contractDiscountSeen && acc.contractDiscountRatio != *row.ContractRatio)
		acc.contractDiscountSeen, acc.contractDiscountRatio = true, *row.ContractRatio
	}
	for marker := range row.Markers {
		acc.priceSnapshotMarkers[marker] = struct{}{}
	}
}

type billingReportCustomerLeaf struct {
	GroupID int64
	Price   billingReportPrice
}

type billingReportCombination struct {
	Combination   BillingDiscountCombination
	Original      decimal.Decimal
	Complete      bool
	FirstSourceID int64
}

type billingReportCustomerDimension struct {
	RecordedAmounts bool
	Leaves          []billingReportCustomerLeaf
	Names           map[int64]string
	NameSource      map[int64]int64
	Combinations    []billingReportCombination
}

type billingReportCustomerDetails struct {
	Key     billingReportCustomerDimension
	Channel billingReportCustomerDimension
}

type billingReportCustomerOverview struct {
	Price          billingReportPrice
	LastActivityAt int64
}

type billingReportCustomerBuild struct {
	key            *billingCustomerSummary
	channel        *billingCustomerSummary
	overview       billingReconciliationModelAccumulator
	lastActivityAt int64
}

func newBillingReportCustomerBuild() *billingReportCustomerBuild {
	out := &billingReportCustomerBuild{
		key: newBillingCustomerSummary(), channel: newBillingCustomerSummary(),
		overview: billingReconciliationModelAccumulator{originalQuotaComplete: true, priceSnapshotMarkers: make(map[string]struct{})},
	}
	// The public combination cap is applied to the merged period, in log-ID
	// order. Publishing a daily overflow would lose identities needed to merge.
	out.key.combinations.limit = 50000
	out.channel.combinations.limit = 50000
	return out
}

func (a *billingReportCustomerBuild) observe(log billingReconciliationLog, parsed parsedBillingReconciliationLog, sourceID int64) error {
	a.key.observe("api_key", log, parsed, sourceID)
	a.channel.observe("channel", log, parsed, sourceID)
	accumulateBillingReconciliationLog(&a.overview.model.Usage, log, parsed)
	accumulateCustomerBillingQuality(&a.overview.model.DataQuality, parsed)
	accumulateBillingReconciliationPrice(&a.overview, log, parsed)
	a.lastActivityAt = max(a.lastActivityAt, log.CreatedAt)
	if a.key.combinations.overflow != nil || a.channel.combinations.overflow != nil {
		return errors.New("billing report combination budget exceeded")
	}
	return nil
}

func projectBillingReportDimension(summary *billingCustomerSummary) billingReportCustomerDimension {
	out := billingReportCustomerDimension{RecordedAmounts: true, Names: summary.names, NameSource: summary.nameSource}
	for key, acc := range summary.models {
		out.Leaves = append(out.Leaves, billingReportCustomerLeaf{key.groupId, projectBillingReportPrice(acc)})
	}
	for _, entry := range summary.combinations.combinations {
		out.Combinations = append(out.Combinations, billingReportCombination{entry.combo, entry.originalQuota, entry.originalComplete, entry.firstSourceID})
	}
	return out
}

func (report *BillingReportManifest) loadCustomerSummary(ctx context.Context, acc *billingCustomerSummary, userID int, dimension string, groupID int, modelName, mode string) error {
	var rows []billingReportCustomerDay
	if err := DB.WithContext(ctx).Select("generation, user_id, leaves").Where("generation IN ? AND user_id = ?", report.ids(), userID).Find(&rows).Error; err != nil {
		return err
	}
	var combinations []billingReportCombination
	for _, row := range rows {
		var details billingReportCustomerDetails
		raw, err := dailyReportPayload(ctx, DB, row.Generation, reportBillingCustomerPayload, row.UserID, row.Leaves)
		if err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(raw, &details); err != nil {
			return err
		}
		view := details.Key
		if dimension == "channel" {
			view = details.Channel
		}
		if !view.RecordedAmounts {
			query := DB.Model(&billingReportLine{}).Where("generation = ? AND user_id = ? AND fund_group <> ?", row.Generation, userID, "")
			if groupID > 0 {
				if dimension == "channel" {
					query = query.Where("channel_id = ?", groupID)
				} else {
					query = query.Where("token_id = ?", groupID)
				}
			}
			query = billingReportLineFilters(query, map[string]string{"customer_model": modelName, "billing_mode": mode})
			if err := view.recoverOriginals(ctx, query, dimension); err != nil {
				return err
			}
		}
		for id, name := range view.Names {
			if view.NameSource[id] >= acc.nameSource[id] {
				acc.names[id], acc.nameSource[id] = name, view.NameSource[id]
			}
		}
		for _, leaf := range view.Leaves {
			model := leaf.Price.Model
			if (groupID > 0 && int64(groupID) != leaf.GroupID) || (modelName != "" && model.ModelName != modelName) || (mode != "" && model.BillingMode != mode) {
				continue
			}
			leaf.Price.mergeInto(acc.model(billingCustomerModelKey{leaf.GroupID, model.ModelName, model.BillingMode}))
		}
		for _, row := range view.Combinations {
			c := row.Combination
			if (groupID <= 0 || int64(groupID) == c.GroupId) && (modelName == "" || c.ModelName == modelName) && (mode == "" || c.BillingMode == mode) {
				combinations = append(combinations, row)
			}
		}
	}
	sort.Slice(combinations, func(i, j int) bool { return combinations[i].FirstSourceID < combinations[j].FirstSourceID })
	for _, row := range combinations {
		key := billingDiscountKeyFromProjection(row.Combination)
		entry := acc.combinations.combinations[key]
		if entry == nil && len(acc.combinations.combinations) >= acc.combinations.limit {
			acc.combinations.observeOverflow(billingReconciliationLog{}, parsedBillingReconciliationLog{})
			accumulateBillingReconciliationUsage(&acc.combinations.overflow.combo.Usage, row.Combination.Usage)
			continue
		}
		if entry == nil {
			entry = &billingDiscountCombinationEntry{combo: row.Combination, originalQuota: row.Original, originalComplete: row.Complete}
			acc.combinations.combinations[key] = entry
			continue
		}
		accumulateBillingReconciliationUsage(&entry.combo.Usage, row.Combination.Usage)
		entry.combo.InputTokensKnown = entry.combo.InputTokensKnown && row.Combination.InputTokensKnown
		entry.combo.EstimateReasons = mergeBillingEstimateReasons(entry.combo.EstimateReasons, row.Combination.EstimateReasons...)
		entry.originalQuota = entry.originalQuota.Add(row.Original)
		entry.originalComplete = entry.originalComplete && row.Complete
	}
	return nil
}

func (report *BillingReportManifest) loadCustomerOverview(ctx context.Context, frozen map[int]BillingCustomerStatementListItem) (map[int]*billingCustomerStatementListAccumulator, error) {
	accumulators := make(map[int]*billingCustomerStatementListAccumulator)
	for _, id := range report.ids() {
		var cursor int
		for {
			var rows []billingReportCustomerDay
			if err := DB.WithContext(ctx).Select("user_id, summary").Where("generation = ? AND user_id > ?", id, cursor).
				Order("user_id ASC").Limit(500).Find(&rows).Error; err != nil {
				return nil, err
			}
			prices := make(map[int]billingReportCustomerOverview, len(rows))
			var legacyUsers []int
			for _, row := range rows {
				if _, ok := frozen[row.UserID]; ok {
					continue
				}
				var data billingReportCustomerOverview
				if err := common.UnmarshalJsonStr(string(row.Summary), &data); err != nil {
					return nil, err
				}
				prices[row.UserID] = data
				if !data.Price.RecordedAmounts {
					legacyUsers = append(legacyUsers, row.UserID)
				}
			}
			if len(legacyUsers) > 0 {
				amounts := make(map[int]billingReportOriginalEvidence, len(legacyUsers))
				query := DB.Model(&billingReportLine{}).Where("generation = ? AND user_id IN ? AND fund_group <> ?", id, legacyUsers, "")
				if err := scanBillingReportCustomerAmounts(ctx, query, func(line billingReportLine, row CustomerExportRow) {
					e := amounts[line.UserID]
					e.observe(row)
					amounts[line.UserID] = e
				}); err != nil {
					return nil, err
				}
				for _, userID := range legacyUsers {
					data := prices[userID]
					amounts[userID].apply(&data.Price)
					prices[userID] = data
				}
			}
			for userID, data := range prices {
				acc := accumulators[userID]
				if acc == nil {
					acc = &billingCustomerStatementListAccumulator{item: BillingCustomerStatementListItem{UserId: userID},
						price: billingReconciliationModelAccumulator{originalQuotaComplete: true, priceSnapshotMarkers: make(map[string]struct{})}}
					accumulators[userID] = acc
				}
				data.Price.mergeInto(&acc.price)
				acc.item.Usage = acc.price.model.Usage
				acc.item.LastActivityAt = max(acc.item.LastActivityAt, data.LastActivityAt)
			}
			if len(rows) < 500 {
				break
			}
			cursor = rows[len(rows)-1].UserID
		}
	}
	return accumulators, nil
}
