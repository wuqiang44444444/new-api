package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// 先取得本月当前确认版，实时扫描排除这些客户；排序、筛选和总计最后统一执行。
func CurrentBillingStatementListItems(ctx context.Context, start, end int64) ([]BillingCustomerStatementListItem, error) {
	rows, err := DB.WithContext(ctx).Table("billing_statement_versions AS v").
		Select("v.id, v.user_id, v.status, v.period_start, v.period_end_exclusive, v.version_number, v.confirmed_at, v.quota_per_unit, v.summary_projection").
		Joins("JOIN billing_statement_months AS m ON m.current_version_id = v.id").
		Where("m.period_start = ?", start).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []BillingCustomerStatementListItem
	for rows.Next() {
		var version BillingStatementVersion
		if err := DB.ScanRows(rows, &version); err != nil {
			return nil, err
		}
		if version.Status != BillingStatementVersionConfirmed || version.PeriodStart != start || version.PeriodEndExclusive != end+1 {
			return nil, ErrBillingStatementVersionConflict
		}
		item, err := frozenBillingStatementListItem(version)
		if err != nil {
			return nil, err
		}
		// Keep only list facts in memory, never all customers' full group projections.
		items = append(items, item)
	}
	return items, rows.Err()
}

func frozenBillingStatementListItem(v BillingStatementVersion) (BillingCustomerStatementListItem, error) {
	var snapshot billingStatementFrozenProjection
	if err := common.UnmarshalJsonStr(string(v.SummaryProjection), &snapshot); err != nil {
		return BillingCustomerStatementListItem{}, err
	}
	s := snapshot.Statement
	if s.DataQuality == nil {
		return BillingCustomerStatementListItem{}, ErrBillingStatementVersionConflict
	}
	item := BillingCustomerStatementListItem{UserId: v.UserId, Username: s.Username, DisplayName: s.DisplayName, Deleted: s.Deleted, Usage: s.Summary, OriginalQuota: s.OriginalQuota, DiscountQuota: s.DiscountQuota, DataQuality: s.DataQuality,
		BillingVersion: &BillingCustomerStatementListVersion{VersionNumber: v.VersionNumber, ConfirmedAt: v.ConfirmedAt}}
	for _, exact := range snapshot.Original {
		value, err := decimal.NewFromString(exact)
		if err != nil {
			return item, err
		}
		item.originalQuota = item.originalQuota.Add(value)
	}
	if v.QuotaPerUnit <= 0 {
		return item, errors.New("invalid frozen quota conversion")
	}
	item.quotaPerUnit = v.QuotaPerUnit
	return item, nil
}

// 跨版本换算参数可能不同；混合列表统一提供 USD 金额，不把不同单位的 quota 当货币相加。
type BillingStatementListMoney struct {
	Gross    string  `json:"gross"`
	Refund   string  `json:"refund"`
	Net      string  `json:"net"`
	Original *string `json:"original"`
	Discount *string `json:"discount"`
}

func billingStatementListMoney(item BillingCustomerStatementListItem) *BillingStatementListMoney {
	unit := item.quotaPerUnit
	if unit <= 0 {
		unit = common.QuotaPerUnit
	}
	convert := func(quota int64) string {
		return decimal.NewFromInt(quota).Div(decimal.NewFromFloat(unit)).StringFixed(8)
	}
	money := &BillingStatementListMoney{Gross: convert(item.Usage.GrossQuota), Refund: convert(item.Usage.RefundQuota), Net: convert(item.Usage.NetQuota)}
	if item.OriginalQuota != nil {
		value := convert(*item.OriginalQuota)
		money.Original = &value
	}
	if item.DiscountQuota != nil {
		value := convert(*item.DiscountQuota)
		money.Discount = &value
	}
	return money
}
func sumBillingStatementListMoney(items []BillingCustomerStatementListItem) *BillingStatementListMoney {
	gross, refund, net, original, discount := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	complete := true
	for _, item := range items {
		m := item.MoneyUSD
		if m == nil {
			return nil
		}
		g, _ := decimal.NewFromString(m.Gross)
		r, _ := decimal.NewFromString(m.Refund)
		n, _ := decimal.NewFromString(m.Net)
		gross = gross.Add(g)
		refund = refund.Add(r)
		net = net.Add(n)
		if m.Original == nil || m.Discount == nil {
			complete = false
			continue
		}
		o, _ := decimal.NewFromString(*m.Original)
		d, _ := decimal.NewFromString(*m.Discount)
		original = original.Add(o)
		discount = discount.Add(d)
	}
	result := &BillingStatementListMoney{Gross: gross.StringFixed(8), Refund: refund.StringFixed(8), Net: net.StringFixed(8)}
	if complete {
		o, d := original.StringFixed(8), discount.StringFixed(8)
		result.Original = &o
		result.Discount = &d
	}
	return result
}
