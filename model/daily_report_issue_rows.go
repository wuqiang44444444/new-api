package model

import (
	"context"
	"time"
)

const dailyReportDiagnosticIssuePredicate = "replay_issue <> '' OR price_issue <> '' OR previous_original <> '' OR previous_original_missing = ?"

func (line *billingReportLine) hasDiagnosticIssue() bool {
	return line.ReplayIssue != "" || line.PriceIssue != "" || line.PreviousOriginal != "" || line.PreviousOriginalMissing
}

// An older process can replace a stage without updating its count columns.
// A count is valid only for the exact frozen generation it describes.
func loadDailyReportIssueRows(ctx context.Context, stage *dailyReportStage) error {
	if stage.Kind != DailyReportBilling || (stage.IssueRows != nil && stage.IssueRowsGeneration == stage.Generation) {
		return nil
	}
	issues, err := countDailyReportDiagnosticRows(ctx, stage.Generation)
	if err != nil {
		return err
	}
	stage.IssueRows, stage.IssueRowsGeneration = &issues, stage.Generation
	return nil
}

// Legacy stages and resumed copies may lack the diagnostic total. Count by
// primary-key pages and yield between them instead of scanning in one query.
func countDailyReportDiagnosticRows(ctx context.Context, generation string) (int64, error) {
	db := usageReportMainDB(ctx)
	var cursor, issues int64
	hasCursor := false
	for {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return 0, err
		}
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		batch := db.WithContext(readCtx).Model(&billingReportLine{}).
			Select("source_id, CASE WHEN "+dailyReportDiagnosticIssuePredicate+" THEN 1 ELSE 0 END AS issue", true).
			Where("generation = ?", generation).Order("source_id").Limit(1000)
		if hasCursor {
			batch = batch.Where("source_id > ?", cursor)
		}
		var summary struct{ Rows, Upper, Issues int64 }
		err := db.WithContext(readCtx).Table("(?) AS copied", batch).
			Select("COUNT(*) AS rows, COALESCE(MAX(source_id), 0) AS upper, COALESCE(SUM(issue), 0) AS issues").Scan(&summary).Error
		cancel()
		if err != nil {
			return 0, err
		}
		issues += summary.Issues
		if summary.Rows < 1000 {
			return issues, nil
		}
		cursor = summary.Upper
		hasCursor = true
	}
}
