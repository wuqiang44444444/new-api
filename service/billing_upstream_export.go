package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

func SubmitUpstreamExportJob(actorID int, request CustomerExportRequest, channelIDs []int, urlKey string) (*model.CustomerExportJob, error) {
	filters, err := normalizeCustomerExportFilters(model.CustomerExportJobTypeStatementDetails, request)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err)
	}
	filters.Upstream, err = model.FreezeUpstreamExportScope(context.Background(), actorID, filters.StartTimestamp, channelIDs, urlKey)
	if err != nil {
		return nil, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err = currentExportObjectStore(); err != nil {
		return nil, err
	}
	// This projection has no cross-customer source revision: never auto-reuse a
	// completed file on the assumption that a log ID proves historical equality.
	job, created, err := model.CreateCustomerExportJob(actorID, 0, model.CustomerExportJobTypeUpstreamDetails, filters)
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
		"Reference amount (after channel discount)",
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
	}
	if filters.Language == "zh" {
		writer.header = []string{
			"任务 ID",
			"生成时间",
			"账期开始（上海时区）",
			"账期结束（不含，上海时区）",
			"URL 归组",
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
			"参考金额（渠道折扣后）",
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
		}
	}
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, GeneratedAt: common.GetTimestamp()}
	filter := model.UpstreamBillingDetailFilter{Start: filters.StartTimestamp, End: filters.EndTimestamp - 1, ChannelIds: filters.Upstream.ChannelIds, ProviderModel: filters.ModelName, BillingMode: filters.BillingMode, RequestId: filters.RequestId, UpstreamRequestId: filters.UpstreamRequestId}
	progress := model.CustomerExportProgress{}
	gate := customerExportSummaryGate(pressure, job, &progress)
	err = model.ScanUpstreamBillingDetails(ctx, filter, filters.Upstream.IncludeRootFields, model.BillingStatementReadPolicy{BatchTimeout: customerExportBatchTimeout, AfterBatch: func(count int) error { progress.Scanned += int64(count); return nil }, BeforeBatch: func(ctx context.Context) error {
		if err := model.UpdateCustomerExportProgress(ctx, job.JobID, job.Executor, progress); err != nil {
			return err
		}
		return gate(ctx)
	}}, func(row model.UpstreamBillingDetailItem) error {
		channel := filters.Upstream.Channels[row.ChannelId]
		reference, discount, version, source, sourceMonth := "", "", "", "", ""
		if d := channel.Discount; d != nil {
			discount, version, source = d.Value.String(), strconv.FormatInt(d.Version, 10), d.Source
			sourceMonth = formatExportTimestamp(d.SourcePeriod)
			if original, e := decimal.NewFromString(row.OriginalExactQuota); e == nil {
				reference = exportCurrencyAmount(original.Mul(d.Value).String(), scope)
			}
		}
		cacheWrite := strconv.FormatInt(row.CacheWriteTokens, 10)
		if row.DataQuality.CacheWriteUnavailableRequests > 0 {
			cacheWrite = ""
		}
		record := []string{
			job.JobID, formatExportTimestamp(scope.GeneratedAt),
			formatExportTimestamp(filters.StartTimestamp), formatExportTimestamp(filters.EndTimestamp),
			exportCsvCellGuard(filters.Upstream.URLKey), strconv.FormatInt(row.RowId, 10),
			formatExportTimestamp(row.Time), strconv.Itoa(row.ChannelId), exportCsvCellGuard(channel.Name),
			exportCsvCellGuard(row.CustomerModel), exportCsvCellGuard(row.ProviderModel),
			exportYesNo(row.ProviderModelFallback), row.BillingMode, row.Event,
			strconv.FormatInt(row.RecordedInputTokens, 10), strconv.FormatInt(row.OutputTokens, 10),
			strconv.FormatInt(row.CacheReadTokens, 10), cacheWrite, row.OriginalExactQuota,
			filters.Currency, strconv.FormatFloat(filters.QuotaPerUnit, 'f', -1, 64),
			strconv.FormatFloat(filters.CurrencyRate, 'f', -1, 64),
			exportCurrencyAmount(row.OriginalExactQuota, scope), reference,
			discount, version, source, sourceMonth,
			exportCsvCellGuard(row.RequestId), exportCsvCellGuard(row.UpstreamRequestId),
			exportCsvCellGuard(row.PlatformTaskId), exportCsvCellGuard(row.UpstreamTaskId),
			strings.Join(row.EstimateReasons, ";"), row.DataQuality.Status,
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
	if source.JobType != model.CustomerExportJobTypeUpstreamDetails {
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
	job, created, err := model.CreateCustomerExportJob(actorID, 0, source.JobType, filters)
	if err != nil {
		return nil, err
	}
	if created {
		_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
	}
	return job, nil
}
