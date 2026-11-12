package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/model"
)

func freezeBillingExportReport(ctx context.Context, filters *model.CustomerExportFilters, version string) (context.Context, error) {
	if len(version) > 1024 {
		return ctx, fmt.Errorf("%w: invalid billing report version", ErrCustomerExportInvalidRequest)
	}
	bound, report, err := model.ResolveBillingReport(ctx, filters.StartTimestamp, filters.EndTimestamp-1, version)
	if err != nil {
		return ctx, err
	}
	filters.BillingReportVersion = report.Version
	filters.BillingReportCoverage = &model.BillingExportCoverage{
		PublishedDays: report.PublishedDays, ExpectedDays: report.ExpectedDays,
		IncompleteDays: report.IncompleteDays, EstimatedAt: report.EstimatedAt,
	}
	return bound, nil
}

const dailyBillingExportFieldVersion = 24

func customerExportFormatVersion(jobType string) int {
	if isDailyBillingExport(jobType) {
		return dailyBillingExportFieldVersion
	}
	switch jobType {
	case model.CustomerExportJobTypeStatementVersion:
		return customerStatementExportFieldVersion
	case model.CustomerExportJobTypeUsageSummary:
		return usageAnalyticsExportFieldVersion
	default:
		return customerExportFieldVersion
	}
}

func billingCoverageCSV(coverage *model.BillingExportCoverage, language string) (header, cells []string) {
	if coverage == nil {
		return nil, nil
	}
	header = []string{"Published report days", "Expected report days", "Incomplete published days", "Provisional data as of (Asia/Shanghai)"}
	if language == "zh" {
		header = []string{"日报已发布天数", "日报应发布天数", "已发布但覆盖不完整的天数", "阶段性数据截止时间（上海时区）"}
	}
	asOf := ""
	if coverage.EstimatedAt > 0 {
		asOf = formatExportTimestamp(coverage.EstimatedAt)
	}
	cells = []string{strconv.Itoa(coverage.PublishedDays), strconv.Itoa(coverage.ExpectedDays), strconv.Itoa(coverage.IncompleteDays), asOf}
	return header, cells
}

func isDailyBillingExport(jobType string) bool {
	switch jobType {
	case model.CustomerExportJobTypeStatementSummary, model.CustomerExportJobTypeStatementDetails,
		model.CustomerExportJobTypeUpstreamSummary, model.CustomerExportJobTypeUpstreamDetails, model.CustomerExportJobTypeCustomerStatements:
		return true
	default:
		return false
	}
}
