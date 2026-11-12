package model

import (
	"context"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Older frozen lines have no verified task relationship. Their stable
// sequence remains an individual group rather than guessing a match.
func billingStatementVersionFundGroupSQL(query *gorm.DB) string {
	cast := "text"
	if query.Dialector.Name() == "mysql" {
		cast = "char"
	}
	return "COALESCE(NULLIF(fund_group, ''), CAST(sequence AS " + cast + "))"
}

func ListBillingStatementVersionFundGroups(ctx context.Context, versionID int64, filter BillingStatementVersionLineFilter, page, size int) ([]BillingStatementFundGroup, int64, error) {
	if page < 1 || size < 1 || size > 500 || page-1 > math.MaxInt/size {
		return nil, 0, common.NewMessage("Invalid time range")
	}
	query := billingStatementVersionLineQuery(ctx, versionID, filter)
	key := billingStatementVersionFundGroupSQL(query)
	var total int64
	if err := query.Session(&gorm.Session{}).Select("COUNT(DISTINCT " + key + ")").Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		BillingStatementFundGroup `gorm:"embedded"`
		OccurredAt                int64
	}
	err := query.Select(key+" AS fund_key, MIN(customer_model) AS model_name, MAX(request_id) AS request_id, COUNT(*) AS event_count, MIN(created_at) AS occurred_at, "+
		"SUM(CASE WHEN log_type = ? THEN -quota ELSE quota END) AS net_quota", LogTypeRefund).
		Group(key).Order("MAX(sequence) DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	result := make([]BillingStatementFundGroup, 0, len(rows))
	for _, row := range rows {
		group := row.BillingStatementFundGroup
		group.Date = time.Unix(row.OccurredAt, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
		result = append(result, group)
	}
	return result, total, nil
}
