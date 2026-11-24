package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func executeUpstreamSummaryExport(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string, pressure *customerExportPressureTracker) (*model.CustomerExportArtifact, error) {
	if pressure == nil {
		pressure = &customerExportPressureTracker{}
	}
	writer, err := newCustomerExportCsvWriter(workDir, "upstream-summary", customerExportCsvShardBytes)
	if err != nil {
		return nil, err
	}
	defer writer.Cleanup()
	language := filters.Language
	writer.header = csvLabels(language, filters.Currency, upstreamSummaryColumns)
	coverage := csvReportCoverage(language, filters.BillingReportCoverage)
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, GeneratedAt: common.GetTimestamp()}
	progress := model.CustomerExportProgress{}
	gate := customerExportSummaryGate(pressure, job, &progress)
	policy := model.BillingStatementReadPolicy{BatchTimeout: customerExportBatchTimeout, MaxGroups: 10000, BeforeBatch: func(ctx context.Context) error {
		if err := model.UpdateCustomerExportProgress(ctx, job.JobID, job.Executor, progress); err != nil {
			return err
		}
		return gate(ctx)
	}, AfterBatch: func(count int) error { progress.Scanned += int64(count); return nil }}
	err = model.ScanUpstreamExportSummary(ctx, filters, policy, func(channelID int, urlKey string, row model.ProviderURLChannelModelSummary) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		channel := filters.Upstream.Channels[channelID]
		coefficient, version := "", ""
		if channel.Discount != nil {
			coefficient = channel.Discount.Value.String()
			if len(channel.Discount.Changes) > 0 {
				parts := []string{formatExportTimestamp(filters.StartTimestamp) + " ×" + coefficient}
				for _, change := range channel.Discount.Changes {
					parts = append(parts, formatExportTimestamp(change.EffectiveFrom)+" ×"+change.Value.String())
				}
				coefficient = strings.Join(parts, " / ")
			}
			version = strconv.FormatInt(channel.Discount.Version, 10)
		}
		var amounts [3]string
		for i, quota := range []*int64{row.OriginalAmount, row.ReferenceAmount, row.KnownReferenceAmount} {
			if quota != nil {
				amounts[i] = exportCurrencyAmount(strconv.FormatInt(*quota, 10), scope)
			}
		}
		usage := row.Usage
		name := filters.Upstream.GroupName
		if filters.Upstream.URLKey == "" {
			name = channel.UpstreamName
		}
		values := map[string]string{
			"channel_id": strconv.Itoa(channelID), "channel": exportCsvCellGuard(channel.Name),
			"name": exportCsvCellGuard(name), "url": exportCsvCellGuard(urlKey),
			"provider_model": exportCsvCellGuard(row.ProviderModel), "mode": customerStatementExportValue(language, "billing_mode", row.BillingMode),
			"reference_amount": amounts[1], "original_amount": amounts[0], "discount": coefficient, "version": version,
			"known_reference_amount": amounts[2], "requests": strconv.FormatInt(usage.Requests, 10),
			"calls": strconv.FormatInt(usage.BillableCalls, 10), "input": strconv.FormatInt(usage.InputTokens, 10),
			"output": strconv.FormatInt(usage.OutputTokens, 10), "cache_read": strconv.FormatInt(usage.CacheReadTokens, 10),
			"images": positiveBillingMeterCSV(usage.BillableImages), "test_images": positiveBillingMeterCSV(usage.TestBillableImages),
			"completed": positiveBillingMeterCSV(usage.CompletedImages),
		}
		var quality string
		if row.DataQuality != nil {
			if row.DataQuality.CacheReadUnavailableRequests > 0 {
				values["cache_read"] = ""
			}
			if row.DataQuality.InputTokensUnavailableRequests > 0 {
				values["input"] = ""
			}
			if evidence := row.DataQuality.EvidenceCoverage; evidence != nil {
				values["gaps"] = strconv.FormatInt(evidence.AmountGapRows, 10)
			}
			quality = upstreamExportQualityReasons(row.DataQuality, language, false)
		}
		if usage.Seconds != nil && usage.SecondsUnavailableRows == 0 {
			values["seconds"] = usage.Seconds.String()
		}
		cacheCells, cacheNote := csvCacheWrites(language, usage.CacheWrites, usage.CacheWriteTokens)
		csvCacheWriteValues(values, cacheCells)
		values["notes"] = csvNotes(coverage, quality, cacheNote, customerExportEstimateReasons(language, row.EstimateReasons),
			csvCountNote(language, usage.SecondsUnavailableRows, "%d records missing billable seconds", "%d 笔缺少计费秒数"),
			csvCountNote(language, usage.BillableImageMissingRows, "%d records missing billable images", "%d 笔缺少计费张数"))
		if err := writer.AppendRecord(csvProjection(upstreamSummaryColumns, values)); err != nil {
			return err
		}
		progress.Matched++
		progress.Written++
		return nil
	})
	if err != nil {
		return nil, err
	}
	files, paths, lines, _, err := writer.Finish()
	if errors.Is(err, errCustomerExportNoFiles) {
		return &model.CustomerExportArtifact{Files: []model.CustomerExportArtifactFile{}, GeneratedAt: scope.GeneratedAt}, nil
	}
	if err != nil {
		return nil, err
	}
	progress.Files = int64(len(files))
	if err := model.UpdateCustomerExportProgress(ctx, job.JobID, job.Executor, progress); err != nil {
		return nil, err
	}
	return uploadCustomerExportArtifact(ctx, job.JobID, files, paths, lines)
}
