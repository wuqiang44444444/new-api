package model

import (
	"time"

	"gorm.io/gorm"
)

// Legacy log pages open the detail dialog from the list payload. They can
// project recorded amounts directly, including from a ClickHouse log source.
func attachUsageLogPageBillingFacts(logs []*Log) error {
	for _, log := range logs {
		if err := attachUsageLogBillingFacts(log, nil); err != nil {
			return err
		}
	}
	return nil
}

// Project customer-safe facts from the authorized log before role formatting.
// Historical recovery uses the published line for the same funding event.
func attachUsageLogBillingFacts(log *Log, reportDB *gorm.DB) error {
	if log.Type != LogTypeConsume {
		return nil
	}
	scanned := customerBillingLogScanRow(log)
	fact := customerExportScanFact(scanned)
	parsed := parseBillingReconciliationLog(fact)
	row := buildCustomerExportRow(scanned, fact, parsed)
	if row.CountsTowardBill && !row.OriginalExact && !parsed.fundingAdjustmentFailed && log.Quota >= 0 && reportDB != nil {
		published, found, err := publishedUsageLogBillingFacts(reportDB, log)
		switch {
		case err != nil:
			row.EstimateReasons = mergeBillingEstimateReasons(row.EstimateReasons, "published_billing_evidence_unavailable")
		case found:
			row.RecordedOriginal = published.RecordedOriginal
			row.OriginalExact = published.OriginalExact
			row.OriginalSource = published.OriginalSource
			row.QualityStatus = published.QualityStatus
			row.EstimateReasons = published.EstimateReasons
		}
	}
	return setBillingLogFacts(log, row)
}

func publishedUsageLogBillingFacts(db *gorm.DB, log *Log) (CustomerExportRow, bool, error) {
	day := time.Unix(log.CreatedAt, 0).In(UsageAnalyticsZone())
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).Unix()
	var line billingReportLine
	result := db.Model(&billingReportLine{}).Select("billing_report_lines.customer").
		Joins("JOIN billing_report_heads AS head ON head.generation = billing_report_lines.generation AND head.start = ?", start).
		Where("source_id = ? AND user_id = ? AND created_at = ? AND log_type = ? AND customer_charge = ? AND fund_group <> ?",
			log.Id, log.UserId, log.CreatedAt, log.Type, log.Quota, "").
		Limit(1).Find(&line)
	if result.Error != nil || result.RowsAffected == 0 {
		return CustomerExportRow{}, false, result.Error
	}
	published, err := line.customerDetail()
	return published.Export, published.Export.CountsTowardBill, err
}
