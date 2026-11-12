package model

import (
	"context"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

const dailyReportPayloadPartSize = 512 * 1024

const (
	reportUsagePayload           = "usage"
	reportBillingCustomerPayload = "customer"
	reportBillingUpstreamPayload = "upstream"
)

type dailyReportPayloadPart struct {
	Generation string `gorm:"primaryKey;size:32"`
	Kind       string `gorm:"primaryKey;size:12"`
	ScopeID    int    `gorm:"primaryKey;autoIncrement:false"`
	Part       int    `gorm:"primaryKey;autoIncrement:false"`
	Payload    BillingStatementJSON
}

// Keep small projections in their indexed row. Larger projections are split
// at UTF-8 boundaries and each write stays under the batch budget.
func writeDailyReportPayload(ctx context.Context, db *gorm.DB, gate func(context.Context) error, generation, kind string, id int, raw []byte) (BillingStatementJSON, error) {
	if len(raw) <= dailyReportPayloadPartSize {
		return BillingStatementJSON(raw), nil
	}
	for part := 0; len(raw) > 0; part++ {
		end := min(len(raw), dailyReportPayloadPartSize)
		for end < len(raw) && !utf8.RuneStart(raw[end]) {
			end--
		}
		row := dailyReportPayloadPart{Generation: generation, Kind: kind, ScopeID: id, Part: part, Payload: BillingStatementJSON(raw[:end])}
		if err := writeBillingReportProjection(ctx, db, gate, end, &row); err != nil {
			return "", err
		}
		raw = raw[end:]
	}
	return "", nil
}

func dailyReportPayload(ctx context.Context, db *gorm.DB, generation, kind string, id int, inline BillingStatementJSON) (string, error) {
	if len(inline) > 0 {
		return string(inline), nil
	}
	var parts []dailyReportPayloadPart
	if err := db.WithContext(ctx).Where("generation = ? AND kind = ? AND scope_id = ?", generation, kind, id).Order("part").Find(&parts).Error; err != nil {
		return "", err
	}
	var out strings.Builder
	for _, part := range parts {
		out.WriteString(string(part.Payload))
	}
	return out.String(), nil
}

// Delete fragments in small writes before the owning generation is removed.
func pruneDailyReportPayload(ctx context.Context, db *gorm.DB, generation string) (bool, error) {
	var parts []dailyReportPayloadPart
	err := db.WithContext(ctx).Where("generation = ?", generation).Select("generation, kind, scope_id, part").Limit(20).Find(&parts).Error
	if err != nil || len(parts) == 0 {
		return false, err
	}
	return true, db.WithContext(ctx).Delete(&parts).Error
}
