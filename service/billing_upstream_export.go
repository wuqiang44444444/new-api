package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

func SubmitUpstreamExportJob(ctx context.Context, actorID int, request CustomerExportRequest, channelIDs []int, urlKey string) (*model.CustomerExportJob, error) {
	jobType := request.JobType
	if jobType == "" {
		jobType = model.CustomerExportJobTypeUpstreamDetails
	}
	if jobType != model.CustomerExportJobTypeUpstreamDetails && jobType != model.CustomerExportJobTypeUpstreamSummary {
		return nil, fmt.Errorf("%w: invalid upstream export type", ErrCustomerExportInvalidRequest)
	}
	if jobType == model.CustomerExportJobTypeUpstreamSummary && (len(channelIDs) == 0 || request.UpstreamEvidenceFilter != "" || request.ModelName != "" || request.BillingMode != "" || request.RequestId != "" || request.UpstreamRequestId != "" || request.ProviderModelFallback != nil) {
		return nil, fmt.Errorf("%w: upstream summary requires one complete channel", ErrCustomerExportInvalidRequest)
	}
	if request.UpstreamAllChannels && (jobType != model.CustomerExportJobTypeUpstreamDetails || len(channelIDs) != 0 || request.ChannelId != nil || urlKey != "" || request.ReportVersion == "") {
		return nil, fmt.Errorf("%w: all-channel export requires a report version and no channel or URL filter", ErrCustomerExportInvalidRequest)
	}
	if err := model.ValidateUpstreamEvidenceFilter(request.UpstreamEvidenceFilter); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err)
	}
	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementDetails, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err)
	}
	filters.FieldVersion = customerExportFormatVersion(jobType)
	if request.ProviderModelFallback != nil && filters.ModelName == "" {
		return nil, fmt.Errorf("%w: provider_model_fallback requires model_name", ErrCustomerExportInvalidRequest)
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	store, err := currentExportObjectStore()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx, err = freezeBillingExportReport(ctx, &filters, request.ReportVersion)
	if err != nil {
		return nil, err
	}
	allChannels := len(channelIDs) == 0 && urlKey == "" && (request.UpstreamEvidenceFilter != "" || request.UpstreamAllChannels)
	filters.Upstream, err = model.FreezeUpstreamExportScope(ctx, actorID, filters.StartTimestamp, channelIDs, urlKey, allChannels)
	if err != nil {
		return nil, err
	}
	if jobType == model.CustomerExportJobTypeUpstreamSummary {
		filters.Upstream.IncludeRootFields = false
	}
	filters.Upstream.EvidenceFilter = request.UpstreamEvidenceFilter
	filters.Upstream.ProviderModelFallback = request.ProviderModelFallback
	job, created, err := model.CreateCustomerExportJob(actorID, 0, jobType, filters,
		model.CustomerExportReuse{StoreIdentity: store.ExportIdentity()})
	if err != nil {
		return nil, err
	}
	if created {
		_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
	}
	return job, nil
}

func executeUpstreamExport(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string, pressure *customerExportPressureTracker) (*model.CustomerExportArtifact, error) {
	if filters.Upstream == nil {
		return nil, errors.New("missing upstream export scope")
	}
	if pressure == nil {
		pressure = &customerExportPressureTracker{}
	}
	writer, err := newCustomerExportCsvWriter(workDir, "upstream-details", customerExportCsvShardBytes)
	if err != nil {
		return nil, err
	}
	defer writer.Cleanup()
	language := filters.Language
	writer.header = csvLabels(language, filters.Currency, upstreamDetailColumns)
	coverage := csvReportCoverage(language, filters.BillingReportCoverage)
	if coverage != "" {
		writer.prefix += "-partial"
	}
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, GeneratedAt: common.GetTimestamp()}
	filter := model.UpstreamBillingDetailFilter{URLKey: filters.Upstream.URLKey, EvidenceFilter: filters.Upstream.EvidenceFilter, Start: filters.StartTimestamp, End: filters.EndTimestamp - 1, ChannelIds: filters.Upstream.ChannelIds, ProviderModel: filters.ModelName, ProviderModelFallback: filters.Upstream.ProviderModelFallback, BillingMode: filters.BillingMode, RequestId: filters.RequestId, UpstreamRequestId: filters.UpstreamRequestId}
	progress := model.CustomerExportProgress{}
	gate := customerExportSummaryGate(pressure, job, &progress)
	err = model.ScanUpstreamBillingDetails(ctx, filter, filters.Upstream.IncludeRootFields, model.BillingStatementReadPolicy{BatchTimeout: customerExportBatchTimeout, AfterBatch: func(count int) error { progress.Scanned += int64(count); return nil }, BeforeBatch: func(ctx context.Context) error {
		if err := model.UpdateCustomerExportProgress(ctx, job.JobID, job.Executor, progress); err != nil {
			return err
		}
		return gate(ctx)
	}}, func(row model.UpstreamBillingDetailItem) error {
		channel := filters.Upstream.Channel(row.ChannelId, filters.StartTimestamp)
		var reference, discount, version string
		if d := channel.Discount; d != nil {
			coefficient := d.ValueAt(row.Time)
			discount, version = coefficient.String(), strconv.FormatInt(d.Version, 10)
			if original, err := decimal.NewFromString(row.OriginalExactQuota); err == nil {
				reference = exportCurrencyAmount(model.UpstreamReferenceQuota(original, coefficient).String(), scope)
			}
		}
		name := filters.Upstream.GroupName
		if filters.Upstream.URLKey == "" {
			name = channel.UpstreamName
		}
		values := map[string]string{
			"time": formatExportTimestamp(row.Time), "channel_id": strconv.Itoa(row.ChannelId),
			"upstream_request": exportCsvCellGuard(row.UpstreamRequestId), "request": exportCsvCellGuard(row.RequestId),
			"event":   customerStatementExportValue(language, "event_type", row.Event),
			"channel": exportCsvCellGuard(channel.Name), "name": exportCsvCellGuard(name),
			"url": exportCsvCellGuard(row.URLKey), "provider_model": exportCsvCellGuard(row.ProviderModel),
			"customer_model": exportCsvCellGuard(row.CustomerModel), "stream": csvStream(language, row.IsStream),
			"reference_amount": reference, "original_amount": exportCurrencyAmount(row.OriginalExactQuota, scope),
			"discount": discount, "version": version, "status": upstreamExportOriginalStatus(row.OriginalAmount, language),
			"mode":  customerStatementExportValue(language, "billing_mode", row.BillingMode),
			"input": strconv.FormatInt(row.RecordedInputTokens, 10), "output": strconv.FormatInt(row.OutputTokens, 10),
			"cache_read": strconv.FormatInt(row.CacheReadTokens, 10), "images": csvOptionalCount(row.BillableImageCount),
			"completed": csvOptionalCount(row.CompletedImageCount), "row_id": strconv.FormatInt(row.RowId, 10),
			"task": exportCsvCellGuard(row.PlatformTaskId), "upstream_task": exportCsvCellGuard(row.UpstreamTaskId),
		}
		if row.Seconds != nil {
			values["seconds"] = row.Seconds.String()
		}
		if row.DataQuality.CacheReadUnavailableRequests > 0 {
			values["cache_read"] = ""
		}
		if row.DataQuality.InputTokensUnavailableRequests > 0 {
			values["input"] = ""
		}
		cacheCells, cacheNote := csvCacheWrites(language, row.CacheWrites, row.CacheWriteTokens)
		csvCacheWriteValues(values, cacheCells)
		var providerNote, replayNote, testNote string
		if row.ProviderModelFallback {
			providerNote = csvText(language, "Upstream model not recorded", "上游模型未记录")
		}
		if replay := row.FrozenReplay; replay != nil {
			replayNote = replay.Reason
			if replay.DeltaQuota != nil && *replay.DeltaQuota != 0 {
				replayNote = csvNotes(replayNote, csvText(language, "Settlement difference (quota) ", "入账差额（额度） ")+strconv.FormatInt(*replay.DeltaQuota, 10))
			}
		}
		if row.TestPricing != nil {
			testNote = csvText(language, "Channel test pricing ", "渠道测试计价 ") + row.TestPricing.Status
		}
		values["notes"] = csvNotes(cacheNote, providerNote, replayNote, testNote, csvDurationNote(language, row.BillableDurations),
			customerExportEstimateReasons(language, row.EstimateReasons), upstreamExportQualityReasons(row.DataQuality, language, true))
		if err := writer.AppendRecord(csvProjection(upstreamDetailColumns, values)); err != nil {
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
	artifact, err := uploadCustomerExportArtifact(ctx, job.JobID, files, paths, lines)
	if err == nil {
		artifact.MissingDates = model.CustomerStatementExportMissingDates(ctx, filters)
	}
	return artifact, err
}

// Resubmission retains the original scope, including channel membership and
// coefficients, and rechecks current authority before creating another job.
func ResubmitUpstreamExportJob(actorID int, sourceJobID string) (*model.CustomerExportJob, error) {
	source, err := model.GetCustomerExportJobForOwner(sourceJobID, actorID)
	if err != nil {
		return nil, err
	}
	if source.JobType != model.CustomerExportJobTypeUpstreamDetails && source.JobType != model.CustomerExportJobTypeUpstreamSummary {
		return nil, model.ErrCustomerExportNotFound
	}
	filters, err := source.DecodeFilters()
	if err != nil {
		return nil, err
	}
	if filters.BillingReportVersion == "" {
		return nil, errCustomerExportFormatChanged
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err = currentExportObjectStore(); err != nil {
		return nil, err
	}
	if filters.BillingReportCoverage == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := freezeBillingExportReport(ctx, &filters, filters.BillingReportVersion); err != nil {
			return nil, err
		}
	}
	filters.FieldVersion = customerExportFormatVersion(source.JobType)
	filters.Language = billingExportLanguage
	job, created, err := model.CreateCustomerExportJob(actorID, 0, source.JobType, filters, model.CustomerExportReuse{RequireExactActiveFilters: true})
	if err != nil {
		return nil, err
	}
	if created {
		_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
	}
	return job, nil
}
