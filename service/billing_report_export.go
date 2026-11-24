package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/model"
)

func freezeBillingExportReport(ctx context.Context, filters *model.CustomerExportFilters, version string) (context.Context, error) {
	if len(version) > billingExportVersionMaxBytes {
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

const billingExportVersionMaxBytes = 1024
const dailyBillingExportFieldVersion = 28
const customerStatementDayExportFieldVersion = 29
const singleCustomerDayExportFieldVersion = 30
const priorDailyBillingExportFieldVersion = 26

// Only formats known to contain the same frozen input can adopt the new
// projection. A staged file must continue using the format that created it.
func upgradeUnstagedBillingExport(ctx context.Context, job *model.CustomerExportJob, filters *model.CustomerExportFilters) error {
	legacyDaily := isDailyBillingExport(job.JobType) && filters.BillingDate == "" &&
		filters.FieldVersion == priorDailyBillingExportFieldVersion && filters.BillingReportVersion != ""
	legacyUsage := job.JobType == model.CustomerExportJobTypeUsageSummary && len(filters.UsageReportDays) > 0 &&
		filters.FieldVersion > 0 && filters.FieldVersion < usageAnalyticsExportFieldVersion
	if (!legacyDaily && !legacyUsage) || job.Artifact != "" {
		return nil
	}
	filters.FieldVersion = customerExportFormatVersion(job.JobType)
	filters.Language = billingExportLanguage
	return model.UpgradeCustomerExportFormat(ctx, job, *filters)
}

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

func isDailyBillingExport(jobType string) bool {
	switch jobType {
	case model.CustomerExportJobTypeStatementSummary, model.CustomerExportJobTypeStatementDetails,
		model.CustomerExportJobTypeUpstreamSummary, model.CustomerExportJobTypeUpstreamDetails, model.CustomerExportJobTypeCustomerStatements:
		return true
	default:
		return false
	}
}
