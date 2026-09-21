package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	if jobType == model.CustomerExportJobTypeUpstreamSummary && (urlKey == "" || len(channelIDs) == 0 || request.UpstreamEvidenceFilter != "" || request.ModelName != "" || request.BillingMode != "" || request.RequestId != "" || request.UpstreamRequestId != "" || request.ProviderModelFallback != nil) {
		return nil, fmt.Errorf("%w: upstream summary requires one complete URL group", ErrCustomerExportInvalidRequest)
	}
	if err := model.ValidateUpstreamEvidenceFilter(request.UpstreamEvidenceFilter); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err)
	}
	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementDetails, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err)
	}
	if request.ProviderModelFallback != nil && filters.ModelName == "" {
		return nil, fmt.Errorf("%w: provider_model_fallback requires model_name", ErrCustomerExportInvalidRequest)
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err = currentExportObjectStore(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	allChannels := len(channelIDs) == 0 && urlKey == "" && request.UpstreamEvidenceFilter != ""
	filters.Upstream, err = model.FreezeUpstreamExportScope(ctx, actorID, filters.StartTimestamp, channelIDs, urlKey, allChannels)
	if err != nil {
		return nil, err
	}
	if jobType == model.CustomerExportJobTypeUpstreamSummary {
		filters.Upstream.IncludeRootFields = false
	}
	filters.Upstream.EvidenceFilter = request.UpstreamEvidenceFilter
	filters.Upstream.ProviderModelFallback = request.ProviderModelFallback
	// This projection has no cross-customer source revision: never auto-reuse a
	// completed file on the assumption that a log ID proves historical equality.
	job, created, err := model.CreateCustomerExportJob(actorID, 0, jobType, filters)
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
	writer.header = []string{
		"Job ID",
		"Generated at",
		"Period start (Asia/Shanghai)",
		"Period end (exclusive, Asia/Shanghai)",
		"URL grouping",
		"Upstream name",
		"Row ID",
		"Time",
		"Channel ID",
		"Channel",
		"Customer model",
		"Provider model",
		"Provider model fallback",
		"Billing mode",
		"Row kind",
		"Input tokens",
		"Output tokens",
		"Cache read tokens",
		"Cache write tokens",
		"Original quota",
		"Currency",
		"Quota per unit",
		"Currency rate",
		"Original amount (local official price)",
		"Calculated amount (after channel discount)",
		"Channel discount",
		"Discount version",
		"Discount source",
		"Source month",
		"Our request ID",
		"Upstream request ID",
		"Platform task ID",
		"Upstream task ID",
		"Estimate reasons",
		"Data status",
		"Billable seconds",
		"Reference quota",
		"Test pricing",
		"Data quality reasons",
		"Seconds basis",
		"Test amount basis", "Recorded test fee", "Recalculated test original",
	}
	if filters.Language == "zh" {
		writer.header = []string{
			"任务 ID",
			"生成时间",
			"账期开始（上海时区）",
			"账期结束（不含，上海时区）",
			"URL 归组",
			"上游名称",
			"记录 ID",
			"时间",
			"渠道 ID",
			"渠道",
			"客户模型",
			"上游模型",
			"上游模型未记录",
			"计费模式",
			"记录类型",
			"输入 Token",
			"输出 Token",
			"缓存读取 Token",
			"缓存写入 Token",
			"本地原价 quota",
			"币种",
			"单位 quota",
			"汇率",
			"原价金额（本地官方价）",
			"核算金额（按渠道折扣）",
			"渠道折扣",
			"折扣版本",
			"折扣来源",
			"来源月份",
			"我方请求 ID",
			"上游请求 ID",
			"平台任务 ID",
			"上游任务 ID",
			"估算原因",
			"数据状态",
			"计费秒数",
			"折后参考 quota",
			"测试计价",
			"数据质量说明",
			"秒数依据",
			"测试金额依据", "原测试记录金额", "复算测试原价",
		}
	}
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, GeneratedAt: common.GetTimestamp()}
	filter := model.UpstreamBillingDetailFilter{EvidenceFilter: filters.Upstream.EvidenceFilter, Start: filters.StartTimestamp, End: filters.EndTimestamp - 1, ChannelIds: filters.Upstream.ChannelIds, ProviderModel: filters.ModelName, ProviderModelFallback: filters.Upstream.ProviderModelFallback, BillingMode: filters.BillingMode, RequestId: filters.RequestId, UpstreamRequestId: filters.UpstreamRequestId}
	progress := model.CustomerExportProgress{}
	gate := customerExportSummaryGate(pressure, job, &progress)
	err = model.ScanUpstreamBillingDetails(ctx, filter, filters.Upstream.IncludeRootFields, model.BillingStatementReadPolicy{UpperLogID: filters.Upstream.UpperLogID, BatchTimeout: customerExportBatchTimeout, AfterBatch: func(count int) error { progress.Scanned += int64(count); return nil }, BeforeBatch: func(ctx context.Context) error {
		if err := model.UpdateCustomerExportProgress(ctx, job.JobID, job.Executor, progress); err != nil {
			return err
		}
		return gate(ctx)
	}}, func(row model.UpstreamBillingDetailItem) error {
		channel := filters.Upstream.Channel(row.ChannelId, filters.StartTimestamp)
		reference, referenceQuota, discount, version, source, sourceMonth := "", "", "", "", "", ""
		if d := channel.Discount; d != nil {
			discount, version, source = d.Value.String(), strconv.FormatInt(d.Version, 10), d.Source
			sourceMonth = formatExportTimestamp(d.SourcePeriod)
			if original, e := decimal.NewFromString(row.OriginalExactQuota); e == nil {
				referenceQuota = model.UpstreamReferenceQuota(original, d.Value).String()
				reference = exportCurrencyAmount(referenceQuota, scope)
			}
		}
		cacheRead := strconv.FormatInt(row.CacheReadTokens, 10)
		if row.DataQuality.CacheReadUnavailableRequests > 0 {
			cacheRead = ""
		}
		cacheWrite := strconv.FormatInt(row.CacheWriteTokens, 10)
		if row.DataQuality.CacheWriteUnavailableRequests > 0 {
			cacheWrite = ""
		}
		secondsBasis := row.SecondsSource
		switch row.SecondsSource {
		case "billing_parameters":
			secondsBasis = "Verified original billing parameters"
			if filters.Language == "zh" {
				secondsBasis = "按当时计价参数复算一致"
			}
		case "refunded_hold":
			secondsBasis = "Customer-refunded task hold"
			if filters.Language == "zh" {
				secondsBasis = "已退还客户的任务预扣"
			}
		case "recorded_usage":
			secondsBasis = "Recorded usage"
			if filters.Language == "zh" {
				secondsBasis = "已记录用量"
			}
		}
		seconds := ""
		if row.Seconds != nil {
			seconds = row.Seconds.String()
		}
		testPricing := ""
		if row.TestPricing != nil {
			testPricing = row.TestPricing.Mode + ":" + row.TestPricing.Status
		}
		testBasis, recordedFee, recomputedFee := "", "", ""
		if row.TestPricing != nil {
			switch row.TestPricing.Basis {
			case "historical_replay":
				testBasis = "Recalculated from recorded prices and usage"
				if filters.Language == "zh" {
					testBasis = "按历史价格与用量复算"
				}
			case "recorded_expression_result":
				testBasis = "Recorded expression result; group multiplier 1"
				if filters.Language == "zh" {
					testBasis = "历史表达式计算金额（分组倍率 1）"
				}
			}
			if row.TestPricing.RecordedQuota != nil {
				recordedFee = exportCurrencyAmount(strconv.FormatInt(*row.TestPricing.RecordedQuota, 10), scope)
			}
			if row.TestPricing.RecomputedQuota != nil {
				recomputedFee = exportCurrencyAmount(strconv.FormatInt(*row.TestPricing.RecomputedQuota, 10), scope)
			}
		}
		record := []string{
			job.JobID, formatExportTimestamp(scope.GeneratedAt),
			formatExportTimestamp(filters.StartTimestamp), formatExportTimestamp(filters.EndTimestamp),
			exportCsvCellGuard(filters.Upstream.URLKey), exportCsvCellGuard(filters.Upstream.GroupName),
			strconv.FormatInt(row.RowId, 10),
			formatExportTimestamp(row.Time), strconv.Itoa(row.ChannelId), exportCsvCellGuard(channel.Name),
			exportCsvCellGuard(row.CustomerModel), exportCsvCellGuard(row.ProviderModel),
			exportYesNo(row.ProviderModelFallback), row.BillingMode, row.Event,
			strconv.FormatInt(row.RecordedInputTokens, 10), strconv.FormatInt(row.OutputTokens, 10),
			cacheRead, cacheWrite, row.OriginalExactQuota,
			filters.Currency, strconv.FormatFloat(filters.QuotaPerUnit, 'f', -1, 64),
			strconv.FormatFloat(filters.CurrencyRate, 'f', -1, 64),
			exportCurrencyAmount(row.OriginalExactQuota, scope), reference,
			discount, version, source, sourceMonth,
			exportCsvCellGuard(row.RequestId), exportCsvCellGuard(row.UpstreamRequestId),
			exportCsvCellGuard(row.PlatformTaskId), exportCsvCellGuard(row.UpstreamTaskId),
			strings.Join(row.EstimateReasons, ";"), row.DataQuality.Status, seconds, referenceQuota, testPricing,
			upstreamExportQualityReasons(row.DataQuality, filters.Language), secondsBasis, testBasis, recordedFee, recomputedFee,
		}
		if err := writer.AppendRecord(record); err != nil {
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
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err = currentExportObjectStore(); err != nil {
		return nil, err
	}
	filters.FieldVersion = customerExportFieldVersion
	job, created, err := model.CreateCustomerExportJob(actorID, 0, source.JobType, filters, model.CustomerExportReuse{RequireExactActiveFilters: true})
	if err != nil {
		return nil, err
	}
	if created {
		_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
	}
	return job, nil
}
