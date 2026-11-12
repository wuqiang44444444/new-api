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
	// Accept only the month, report version and language for an all-customer job.
	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementSummary, CustomerExportRequest{
		StartTimestamp: request.StartTimestamp,
		EndTimestamp:   request.EndTimestamp,
		Language:       request.Language,
	})
	if err != nil {
		return nil, ErrCustomerExportInvalidRequest
	}
	filters.FieldVersion = customerExportFormatVersion(model.CustomerExportJobTypeCustomerStatements)
	filters.Currency, filters.CurrencyRate = "USD", 1
	ctx, cancel := context.WithTimeout(ctx, customerExportBatchTimeout)
	defer cancel()
	if _, err := freezeBillingExportReport(ctx, &filters, request.ReportVersion); err != nil {
		return nil, err
	}
	job, created, err := model.CreateCustomerExportJob(actorID, 0, model.CustomerExportJobTypeCustomerStatements, filters)
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
	confirmed, err := model.CurrentBillingStatementListItems(ctx, filters.StartTimestamp, end)
	if err != nil {
		return nil, err
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
	month := time.Unix(filters.StartTimestamp, 0).In(billingExportTimezone()).Format("2006-01")
	writer, err := newCustomerExportCsvWriter(workDir, "customer-statements-"+month, customerExportCsvShardBytes)
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
	writer.header = []string{"Billing month (Asia/Shanghai)", "Customer ID", "Username", "Customer name", "Requests", "Original amount (USD)", "Savings (USD)", "Total charges (USD)", "Total returns (USD)", "Net settled amount (USD)", "Data quality", "Confirmed statement version"}
	if filters.Language == "zh" {
		writer.header = []string{"账单月份（上海时区）", "客户 ID", "用户名", "客户名称", "请求数", "原价（USD）", "优惠（USD）", "总扣减（USD）", "总退回（USD）", "净结算金额（USD）", "数据质量", "确认账单版本"}
	}
	coverageHeader, coverageCells := billingCoverageCSV(filters.BillingReportCoverage, filters.Language)
	writer.header = append(writer.header, coverageHeader...)
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
		quality, version := "", ""
		if item.DataQuality != nil {
			quality = customerStatementExportValue(filters.Language, "quality_status", item.DataQuality.Status)
		}
		if item.BillingVersion != nil && item.BillingVersion.VersionNumber != nil {
			version = strconv.Itoa(*item.BillingVersion.VersionNumber)
		}
		record := []string{
			month, strconv.Itoa(item.UserId), exportCsvCellGuard(item.Username), exportCsvCellGuard(item.DisplayName),
			strconv.FormatInt(item.Usage.Requests, 10), original, savings, gross, returns, net, quality, version,
		}
		if item.BillingVersion != nil {
			record = append(record, make([]string, len(coverageCells))...)
		} else {
			record = append(record, coverageCells...)
		}
		if err := writer.AppendRecord(record); err != nil {
			return nil, nil, 0, err
		}
	}
	files, paths, lines, _, err := writer.Finish()
	finished = err == nil
	return files, paths, lines, err
}
