package model

import (
	"context"
	"encoding/base64"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"gorm.io/gorm"
)

type BillingReportDiagnosticDay struct {
	Date          string `json:"date"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Source        string `json:"source,omitempty"`
	IssueRows     int64  `json:"issue_rows"`
	PublishedRows *int64 `json:"published_source_rows,omitempty"`
	CandidateRows *int64 `json:"candidate_source_rows,omitempty"`
	RetryAt       int64  `json:"retry_at,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type BillingReportDiagnosticLine struct {
	SourceID                int64  `json:"source_id"`
	CustomerModel           string `json:"model"`
	CustomerCharge          int64  `json:"net_quota"`
	PreviousOriginalMissing bool   `json:"previous_original_missing,omitempty"`
	PreviousOriginal        string `json:"previous_original,omitempty"`
	CurrentOriginal         string `json:"current_original,omitempty"`
	ReplayIssue             string `json:"replay_issue,omitempty"`
	PriceIssue              string `json:"price_issue,omitempty"`
}

func BillingReportDiagnosticDays(ctx context.Context, start, end int64) ([]BillingReportDiagnosticDay, error) {
	db := DB.WithContext(ctx)
	days := make([]BillingReportDiagnosticDay, 0)
	var retries []dailyReportRetry
	if err := db.Where("start >= ? AND start < ? AND "+currentDailyReportRetry, start, end, usageReportMeterVersion, CustomerOriginalReplayVersion).
		Find(&retries).Error; err != nil {
		return nil, err
	}
	pending := make(map[DailyReportScope]dailyReportRetry, len(retries))
	for _, retry := range retries {
		pending[DailyReportScope{Start: retry.Start, Kind: retry.Kind}] = retry
	}
	for _, kind := range []string{DailyReportBilling, DailyReportUsage} {
		var heads []struct {
			Start            int64
			Generation       string
			Version          int
			HistoricalSource string
			IssueRows        int64
			PublishedRows    int64
			CandidateRows    *int64
		}
		current := usageReportMeterVersion
		query := db.Table("usage_report_days AS h").Select("h.start, h.generation, COALESCE(h.meter_version, 0) AS version, h.historical_source, h.source_rows AS published_rows, s.source_rows AS candidate_rows")
		if kind == DailyReportBilling {
			current = CustomerOriginalReplayVersion
			query = db.Table("billing_report_heads AS h").
				Select("h.start, h.generation, COALESCE(g.original_replay_version, 0) AS version, g.historical_source, g.issue_rows, g.rows AS published_rows, s.source_rows AS candidate_rows").
				Joins("JOIN billing_report_generations AS g ON g.id = h.generation")
		}
		query = query.Joins("LEFT JOIN daily_report_stages AS s ON s.start = h.start AND s.kind = ? AND s.previous = h.generation AND (s.copying_generation IS NULL OR s.copying_generation <> s.generation)", kind)
		if err := query.Where("h.start >= ? AND h.start < ?", start, end).Order("h.start").Scan(&heads).Error; err != nil {
			return nil, err
		}
		for _, head := range heads {
			key := DailyReportScope{Start: head.Start, Kind: kind}
			retry, queued := pending[key]
			delete(pending, key)
			status := "current"
			switch {
			case retry.CoverageIssue:
				status = "coverage_issue"
			case queued:
				status = "retrying"
			case head.Version != current:
				status = "rebuilding"
			case head.HistoricalSource != "" || head.IssueRows > 0:
				status = "review"
			}
			result := BillingReportDiagnosticDay{
				Date: time.Unix(head.Start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly),
				Kind: kind, Status: status, Source: head.HistoricalSource, IssueRows: head.IssueRows,
				RetryAt: retry.RetryAt, FailureReason: retry.FailureReason,
			}
			if retry.CoverageIssue {
				candidate := retry.CandidateRows
				if candidate == nil {
					candidate = head.CandidateRows
				}
				if candidate != nil {
					result.PublishedRows, result.CandidateRows = &head.PublishedRows, candidate
				}
			}
			days = append(days, result)
		}
	}
	for _, retry := range pending {
		status := "retrying"
		if retry.CoverageIssue {
			status = "coverage_issue"
		}
		days = append(days, BillingReportDiagnosticDay{
			Date: time.Unix(retry.Start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly),
			Kind: retry.Kind, Status: status, CandidateRows: retry.CandidateRows, RetryAt: retry.RetryAt, FailureReason: retry.FailureReason,
		})
	}
	sort.Slice(days, func(i, j int) bool {
		if days[i].Kind != days[j].Kind {
			return days[i].Kind < days[j].Kind
		}
		return days[i].Date < days[j].Date
	})
	return days, nil
}

func BillingReportDiagnosticLines(ctx context.Context, start, after int64) ([]BillingReportDiagnosticLine, error) {
	var lines []billingReportLine
	err := DB.WithContext(ctx).Table("billing_report_lines AS l").Select("l.*").
		Joins("JOIN billing_report_heads AS h ON h.generation = l.generation").
		Joins("JOIN billing_report_generations AS g ON g.id = h.generation").
		Where("h.start = ? AND g.original_replay_version = ? AND l.source_id > ?", start, CustomerOriginalReplayVersion, after).
		Where("l.replay_issue <> '' OR l.price_issue <> '' OR l.previous_original <> '' OR l.previous_original_missing = ?", true).
		Order("l.source_id").Limit(100).Scan(&lines).Error
	if err != nil {
		return nil, err
	}
	result := make([]BillingReportDiagnosticLine, 0, len(lines))
	for _, line := range lines {
		var row billingReportCustomerLine
		if line.Customer != "" {
			row, err = line.customerDetail()
			if err != nil {
				return nil, err
			}
		}
		result = append(result, BillingReportDiagnosticLine{SourceID: line.SourceID, CustomerModel: line.CustomerModel,
			CustomerCharge: line.CustomerCharge, PreviousOriginal: line.PreviousOriginal, PreviousOriginalMissing: line.PreviousOriginalMissing, CurrentOriginal: row.Export.RecordedOriginal,
			ReplayIssue: line.ReplayIssue, PriceIssue: line.PriceIssue})
	}
	return result, nil
}

// Diagnose the price that was actually written to the log. The current model
// configuration and provider telemetry cannot establish historical price inputs.
func historicalImagePriceIssue(log Log, parsed parsedBillingReconciliationLog, drafts map[string]string) string {
	if log.Type != LogTypeConsume || parsed.billingMode != BillingReconciliationModePerImage {
		return ""
	}
	facts := loadCustomerExportFacts(log.Other)
	encoded := billingBreakdownString(facts.raw("expr_b64"))
	if encoded == "" {
		return ""
	}
	draft, seen := drafts[encoded]
	if !seen {
		if body, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			if table := billingexpr.ReadLegacyImageSizePrices(string(body)); table != nil {
				draft, _ = billingexpr.GenerateImageSizePrices(table.Profile, table.Prices)
			}
		}
		drafts[encoded] = draft
	}
	if draft == "" {
		return ""
	}
	var messages []common.Message
	if common.Unmarshal(facts.other["content_parts"], &messages) != nil {
		return "image_price_inputs_missing"
	}
	var size, quality string
	for _, message := range messages {
		switch message.Key {
		case common.ImageSizeLogMessage:
			size, _ = message.Params["size"].(string)
		case common.ImageQualityLogMessage:
			quality, _ = message.Params["quality"].(string)
		}
	}
	if size == "" || quality == "" {
		return "image_price_inputs_missing"
	}
	body, err := common.Marshal(map[string]string{"size": size, "quality": quality})
	if err != nil {
		return "image_price_inputs_missing"
	}
	_, trace, err := billingexpr.RunExprWithRequest(draft, billingexpr.TokenParams{}, billingexpr.RequestInput{Body: body})
	fixed, ok := billingReconciliationFloat(facts.raw("fixed_price"))
	frozenTier := billingBreakdownString(facts.raw("matched_tier"))
	if err != nil || trace.FixedPrice == nil || !ok || frozenTier == "" {
		return "image_price_inputs_missing"
	}
	if trace.MatchedTier != frozenTier && fixed != *trace.FixedPrice {
		return "image_price_tier_mismatch"
	}
	return ""
}

// Carry the baseline of a correction in the same batch insert as the new
// projection. The published source rows and publication checks remain read-only.
func attachPreviousBillingOriginals(db *gorm.DB, previous string, rows []billingReportLine) error {
	if previous == "" {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Customer != "" {
			ids = append(ids, row.SourceID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var prior []billingReportLine
	if err := db.Select("source_id", "customer", "previous_original", "previous_original_missing").Where("generation = ? AND source_id IN ?", previous, ids).Find(&prior).Error; err != nil {
		return err
	}
	byID := make(map[int64]billingReportLine, len(prior))
	for _, row := range prior {
		byID[row.SourceID] = row
	}
	for i := range rows {
		row := &rows[i]
		old, ok := byID[row.SourceID]
		if !ok || old.Customer == "" {
			continue
		}
		if old.PreviousOriginalMissing || old.PreviousOriginal != "" {
			row.PreviousOriginal, row.PreviousOriginalMissing = old.PreviousOriginal, old.PreviousOriginalMissing
			continue
		}
		before, err := old.customerDetail()
		if err != nil {
			return err
		}
		after, err := row.customerDetail()
		if err != nil {
			return err
		}
		if before.Export.RecordedOriginal != after.Export.RecordedOriginal {
			row.PreviousOriginal = before.Export.RecordedOriginal
			row.PreviousOriginalMissing = row.PreviousOriginal == ""
		}
	}
	return nil
}
