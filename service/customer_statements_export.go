package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

func SubmitCustomerStatementsExport(ctx context.Context, actorID int, request CustomerExportRequest) (*model.CustomerExportJob, error) {
	if err := model.AuthorizeCustomerStatementsExport(ctx, actorID); err != nil {
		return nil, err
	}
	if !model.DailyReportSourceSupported() {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err := currentExportObjectStore(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, customerExportBatchTimeout)
	defer cancel()
	var filters model.CustomerExportFilters
	var reportCtx context.Context
	var err error
	if request.SourceJobID != "" {
		if request.BillingDate != "" || request.ReportVersion != "" || request.ExpectedIncomplete != nil ||
			request.StartTimestamp != 0 || request.EndTimestamp != 0 || request.Language != "" {
			return nil, ErrCustomerExportInvalidRequest
		}
		source, err := model.GetCustomerExportJobForOwner(request.SourceJobID, actorID)
		if err != nil {
			return nil, err
		}
		filters, err = source.DecodeFilters()
		if err != nil || source.JobType != model.CustomerExportJobTypeCustomerStatements || source.TargetUserId != 0 ||
			filters.BillingDate == "" || filters.BillingReportCoverage == nil {
			return nil, ErrCustomerExportInvalidRequest
		}
		filters.FieldVersion = customerStatementDayExportFieldVersion
		filters.Language = billingExportLanguage
		reportCtx, _, err = model.ResolveBillingReport(ctx, filters.StartTimestamp, filters.EndTimestamp-1, filters.BillingReportVersion)
		if err != nil {
			return nil, err
		}
	} else {
		// The scope includes every customer's statement for the selected period.
		filters, err = normalizeCustomerExportFilters(model.CustomerExportJobTypeCustomerStatements, CustomerExportRequest{
			BillingDate:    request.BillingDate,
			StartTimestamp: request.StartTimestamp,
			EndTimestamp:   request.EndTimestamp,
			Language:       request.Language,
		})
		if err != nil {
			return nil, ErrCustomerExportInvalidRequest
		}
		daily := filters.BillingDate != ""
		if daily != (request.ExpectedIncomplete != nil) || (daily && len(request.ReportVersion) != model.BillingReportGenerationIDLength) {
			return nil, ErrCustomerExportInvalidRequest
		}
		filters.Currency, filters.CurrencyRate = "USD", 1
		reportCtx, err = freezeBillingExportReport(ctx, &filters, request.ReportVersion)
		if err != nil {
			return nil, err
		}
	}
	var job *model.CustomerExportJob
	var created bool
	if filters.BillingDate != "" {
		report := model.BillingReportFromContext(reportCtx)
		calendar, calendarErr := report.CustomerStatementExportCalendar(reportCtx, filters.StartTimestamp, filters.EndTimestamp)
		if calendarErr != nil {
			return nil, calendarErr
		}
		if len(calendar.Days) != 1 || calendar.Days[0].ReportVersion == "" ||
			calendar.Days[0].ReportVersion != filters.BillingReportVersion {
			return nil, common.NewMessage("The selected billing date is unavailable.")
		}
		job, created, err = model.CreateCustomerStatementDayExportJob(actorID, filters, request.ExpectedIncomplete)
	} else {
		job, created, err = model.CreateCustomerExportJob(actorID, 0, model.CustomerExportJobTypeCustomerStatements, filters)
	}
	if err != nil {
		return nil, err
	}
	if created {
		_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
	}
	return job, nil
}

func executeCustomerStatementsExport(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string) (*model.CustomerExportArtifact, error) {
	if model.BillingReportFromContext(ctx) == nil {
		return nil, errCustomerExportFormatChanged
	}
	end := filters.EndTimestamp - 1
	var confirmed []model.BillingCustomerStatementListItem
	if filters.BillingDate == "" {
		var err error
		confirmed, err = model.CurrentBillingStatementListItems(ctx, filters.StartTimestamp, end)
		if err != nil {
			return nil, err
		}
	}
	// The list already collects and sorts all matching customers before paging.
	// Read it once so a large export does not repeat the aggregation for each page.
	list, err := model.GetBillingCustomerStatementList(ctx, filters.StartTimestamp, end, "", "", "net_quota", "desc", 1, math.MaxInt, confirmed...)
	if err != nil {
		return nil, err
	}
	generatedAt := common.GetTimestamp()
	files, paths, lines, err := writeCustomerStatementsCSV(ctx, workDir, filters, list.Items)
	if errors.Is(err, errCustomerExportNoFiles) {
		return &model.CustomerExportArtifact{Files: []model.CustomerExportArtifactFile{}, GeneratedAt: generatedAt}, nil
	}
	if err != nil {
		return nil, err
	}
	artifact, err := uploadCustomerExportArtifact(ctx, job.JobID, files, paths, lines)
	if err == nil {
		artifact.GeneratedAt = generatedAt
	}
	return artifact, err
}

func writeCustomerStatementsCSV(ctx context.Context, workDir string, filters model.CustomerExportFilters, items []model.BillingCustomerStatementListItem) ([]model.CustomerExportArtifactFile, []string, int64, error) {
	period := filters.BillingDate
	if period == "" {
		period = time.Unix(filters.StartTimestamp, 0).In(billingExportTimezone()).Format("2006-01")
	}
	writer, err := newCustomerExportCsvWriter(workDir, "customer-statements-"+period, customerExportCsvShardBytes)
	if err != nil {
		return nil, nil, 0, err
	}
	// Leave finished shards for the runner to upload. On failure, close any
	// open file before returning control to the runner.
	finished := false
	defer func() {
		if !finished {
			writer.Cleanup()
		}
	}()
	csvLang := filters.Language
	periodColumn := csvText(csvLang, "Billing month (Asia/Shanghai)", "账单月份（上海时区）")
	if filters.BillingDate != "" {
		periodColumn = csvText(csvLang, "Billing date (Asia/Shanghai)", "账单日期（上海时区）")
	}
	writer.header = []string{periodColumn, csvText(csvLang, "Customer ID", "客户 ID"), csvText(csvLang, "Customer", "客户"),
		csvText(csvLang, "Net settled amount (USD)", "净结算金额（USD）"), csvText(csvLang, "Total charges (USD)", "总扣减（USD）"),
		csvText(csvLang, "Total returns (USD)", "总退回（USD）"), csvText(csvLang, "Original amount (USD)", "原价（USD）"),
		csvText(csvLang, "Savings (USD)", "优惠（USD）"), csvText(csvLang, "Requests", "请求数"),
		csvText(csvLang, "Statement version", "账单版本"), csvText(csvLang, "Reconciliation notes", "对账说明")}
	coverage := csvReportCoverage(csvLang, filters.BillingReportCoverage)
	quotaPerUnit := decimal.NewFromFloat(filters.QuotaPerUnit)
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, nil, 0, err
		}
		original, savings := "", ""
		var gross, returns, net string
		if item.BillingVersion != nil && item.MoneyUSD != nil {
			gross, returns, net = item.MoneyUSD.Gross, item.MoneyUSD.Refund, item.MoneyUSD.Net
			if item.MoneyUSD.Original != nil {
				original = *item.MoneyUSD.Original
			}
			if item.MoneyUSD.Discount != nil {
				savings = *item.MoneyUSD.Discount
			}
		} else {
			money := func(quota int64) string {
				return decimal.NewFromInt(quota).Div(quotaPerUnit).StringFixed(8)
			}
			gross, returns, net = money(item.Usage.GrossQuota), money(item.Usage.RefundQuota), money(item.Usage.NetQuota)
			if item.OriginalQuota != nil {
				original = money(*item.OriginalQuota)
			}
			if item.DiscountQuota != nil {
				savings = money(*item.DiscountQuota)
			}
		}
		var version, coverageNote, quality string
		if item.BillingVersion == nil {
			coverageNote = coverage
		}
		if item.DataQuality != nil && item.DataQuality.Status == "partial" {
			quality = customerStatementExportValue(csvLang, "quality_status", item.DataQuality.Status)
		}
		if filters.BillingDate != "" {
			version = filters.BillingReportVersion
		} else if item.BillingVersion != nil && item.BillingVersion.VersionNumber != nil {
			version = strconv.Itoa(*item.BillingVersion.VersionNumber)
		}
		name := item.DisplayName
		if name == "" {
			name = item.Username
		}
		if original == "" {
			savings = ""
		}
		record := []string{
			period, strconv.Itoa(item.UserId), exportCsvCellGuard(name), net, gross, returns, original, savings,
			strconv.FormatInt(item.Usage.Requests, 10), version, csvNotes(coverageNote, quality, csvMissingOriginal(csvLang, original)),
		}
		if err := writer.AppendRecord(record); err != nil {
			return nil, nil, 0, err
		}
	}
	files, paths, lines, _, err := writer.Finish()
	finished = err == nil
	return files, paths, lines, err
}
