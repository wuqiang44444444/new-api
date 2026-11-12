package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// UsageReportNonToken holds independent units from the same frozen report as the rank.
type UsageReportNonToken struct {
	ImageCount              int64            `json:"image_count"`
	ImageCountKnown         bool             `json:"image_count_known,omitempty"`
	ImageCountMissingRows   int64            `json:"image_count_missing_rows,omitempty"`
	Seconds                 *decimal.Decimal `json:"seconds,omitempty"`
	SecondsMissingRows      int64            `json:"seconds_missing_rows,omitempty"`
	SecondsValueMissingRows int64            `json:"seconds_value_missing_rows,omitempty"`
}

func usageReportNonToken(metrics UsageAnalyticsMetrics) *UsageReportNonToken {
	return &UsageReportNonToken{
		ImageCount: metrics.ImageCount, ImageCountKnown: metrics.ImageCountKnown,
		ImageCountMissingRows: metrics.ImageCountMissingRows, Seconds: metrics.Seconds,
		SecondsMissingRows: metrics.SecondsMissingRows, SecondsValueMissingRows: metrics.SecondsValueMissingRows,
	}
}

func (m *UsageReportNonToken) merge(day *UsageReportNonToken) {
	m.ImageCount += day.ImageCount
	m.ImageCountKnown = m.ImageCountKnown || day.ImageCountKnown
	m.ImageCountMissingRows += day.ImageCountMissingRows
	m.SecondsMissingRows += day.SecondsMissingRows
	m.SecondsValueMissingRows += day.SecondsValueMissingRows
	if day.Seconds != nil {
		if m.Seconds == nil {
			m.Seconds = day.Seconds
		} else {
			sum := m.Seconds.Add(*day.Seconds)
			m.Seconds = &sum
		}
	}
}

func attachUsageRankNonToken(ctx context.Context, scope *gorm.DB, period UsageAnalyticsPeriod, filter UsageReportRankQuery, refs []UsageReportDayReference, ranked []usageReportRank) error {
	if len(ranked) == 0 {
		return nil
	}
	subjects := make([]string, 0, len(ranked))
	bySubject := make(map[string]*UsageReportNonToken, len(ranked))
	for i := range ranked {
		row := &ranked[i]
		row.NonToken = &UsageReportNonToken{}
		subjects = append(subjects, row.Subject)
		bySubject[row.Subject] = row.NonToken
	}
	// Page before loading metering payloads. Decimal addition stays exact on
	// all supported databases, including SQLite.
	var days []usageReportRank
	if err := scope.Select("generation, subject, non_token_payload").Where("subject IN ?", subjects).Scan(&days).Error; err != nil {
		return err
	}
	legacy := make(map[string]bool)
	for _, day := range days {
		if day.NonTokenPayload == "" {
			legacy[day.Generation] = true
			continue
		}
		var meters UsageReportNonToken
		if err := common.UnmarshalJsonStr(day.NonTokenPayload, &meters); err != nil {
			return err
		}
		bySubject[day.Subject].merge(&meters)
	}
	if len(legacy) == 0 {
		return nil
	}
	oldRefs := make([]UsageReportDayReference, 0, len(legacy))
	for _, ref := range refs {
		if legacy[ref.Generation] {
			oldRefs = append(oldRefs, ref)
		}
	}
	rows, err := readLegacyUsageRanks(WithUsageReportReferences(ctx, oldRefs), period, filter)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if meters := bySubject[usageRankSubject(row.Name, row.ChannelID, row.BillingMode)]; meters != nil {
			meters.merge(row.NonToken)
		}
	}
	return nil
}
