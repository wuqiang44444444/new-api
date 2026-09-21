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
	writer.header = []string{
		"Job ID",
		"Period start (Asia/Shanghai)",
		"Period end (exclusive, Asia/Shanghai)",
		"URL grouping",
		"Upstream name",
		"Channel ID",
		"Channel",
		"Provider model",
		"Provider model fallback",
		"Billing mode",
		"Input tokens",
		"Cache read tokens",
		"Cache write tokens",
		"Output tokens",
		"Requests",
		"Billable calls",
		"Original amount (local official price)",
		"Calculated amount (after channel discount)",
		"Channel discount",
		"Discount version",
		"Data status",
		"Generated at",
		"Currency",
		"Quota per unit",
		"Currency rate",
		"Billable seconds",
		"Data quality reasons",
		"Known original subtotal",
		"Known reference subtotal",
		"Billing evidence records",
		"Records needing review (deduplicated)",
		"Records without these evidence gaps",
		"Seconds unavailable rows",
		"Customer models",
		"URL grouping basis",
	}
	if filters.Language == "zh" {
		writer.header = []string{
			"任务 ID",
			"账期开始（上海时区）",
			"账期结束（不含，上海时区）",
			"URL 归组",
			"上游名称",
			"渠道 ID",
			"渠道",
			"上游模型",
			"上游模型未记录",
			"计费模式",
			"输入 Token",
			"缓存读取 Token",
			"缓存写入 Token",
			"输出 Token",
			"请求数",
			"计费调用数",
			"原价金额（本地官方价）",
			"核算金额（按渠道折扣）",
			"渠道折扣",
			"折扣版本",
			"数据状态",
			"生成时间",
			"币种",
			"单位 quota",
			"汇率",
			"计费秒数",
			"数据质量说明",
			"已知原价小计",
			"已知核算小计",
			"账单证据记录数",
			"待核查记录数（去重）",
			"无上述证据缺项的记录数",
			"缺少计费秒数的记录数",
			"客户模型",
			"归组依据",
		}
	}
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, GeneratedAt: common.GetTimestamp()}
	progress := model.CustomerExportProgress{}
	gate := customerExportSummaryGate(pressure, job, &progress)
	policy := model.BillingStatementReadPolicy{BatchTimeout: customerExportBatchTimeout, MaxGroups: 10000, BeforeBatch: func(ctx context.Context) error {
		if err := model.UpdateCustomerExportProgress(ctx, job.JobID, job.Executor, progress); err != nil {
			return err
		}
		return gate(ctx)
	}, AfterBatch: func(count int) error { progress.Scanned += int64(count); return nil }}
	err = model.ScanUpstreamExportSummary(ctx, filters, policy, func(channelID int, row model.ProviderURLChannelModelSummary) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		channel := filters.Upstream.Channels[channelID]
		coefficient, version := "", ""
		if channel.Discount != nil {
			coefficient = channel.Discount.Value.String()
			version = strconv.FormatInt(channel.Discount.Version, 10)
		}
		amounts := make([]string, 4)
		for i, quota := range []*int64{row.OriginalAmount, row.ReferenceAmount, row.KnownOriginalAmount, row.KnownReferenceAmount} {
			if quota != nil {
				amounts[i] = exportCurrencyAmount(strconv.FormatInt(*quota, 10), scope)
			}
		}
		read, write := strconv.FormatInt(row.Usage.CacheReadTokens, 10), strconv.FormatInt(row.Usage.CacheWriteTokens, 10)
		quality, reasons, rows, gaps, complete := "complete", "", "", "", ""
		if row.DataQuality != nil {
			quality = row.DataQuality.Status
			reasons = upstreamExportQualityReasons(row.DataQuality, filters.Language)
			if row.DataQuality.CacheReadUnavailableRequests > 0 {
				read = ""
			}
			if row.DataQuality.CacheWriteUnavailableRequests > 0 {
				write = ""
			}
			if coverage := row.DataQuality.EvidenceCoverage; coverage != nil {
				rows, gaps, complete = strconv.FormatInt(coverage.Rows, 10), strconv.FormatInt(coverage.GapRows, 10), strconv.FormatInt(coverage.Rows-coverage.GapRows, 10)
			}
		}
		seconds := ""
		if row.Usage.Seconds != nil {
			seconds = row.Usage.Seconds.String()
		}
		if len(row.EstimateReasons) > 0 {
			reasons = strings.TrimSpace(reasons + " " + strings.Join(row.EstimateReasons, ";"))
		}
		grouping := "Current channel base URL"
		if filters.Language == "zh" {
			grouping = "按渠道当前基础 URL 归组"
		}
		if strings.HasPrefix(filters.Upstream.URLKey, "channel:") {
			grouping = "Unidentified URL — kept per channel"
			if filters.Language == "zh" {
				grouping = "无法识别 URL，按渠道单独保留"
			}
		}
		record := []string{
			job.JobID,
			formatExportTimestamp(filters.StartTimestamp),
			formatExportTimestamp(filters.EndTimestamp),
			exportCsvCellGuard(filters.Upstream.URLKey),
			exportCsvCellGuard(filters.Upstream.GroupName),
			strconv.Itoa(channelID),
			exportCsvCellGuard(channel.Name),
			exportCsvCellGuard(row.ProviderModel),
			exportYesNo(row.ProviderModelFallback),
			customerStatementExportValue(filters.Language, "billing_mode", row.BillingMode),
			strconv.FormatInt(row.Usage.InputTokens, 10),
			read,
			write,
			strconv.FormatInt(row.Usage.OutputTokens, 10),
			strconv.FormatInt(row.Usage.Requests, 10),
			strconv.FormatInt(row.Usage.BillableCalls, 10),
			amounts[0],
			amounts[1],
			coefficient,
			version,
			customerStatementExportValue(filters.Language, "quality_status", quality),
			formatExportTimestamp(scope.GeneratedAt),
			filters.Currency,
			strconv.FormatFloat(filters.QuotaPerUnit, 'f', -1, 64),
			strconv.FormatFloat(filters.CurrencyRate, 'f', -1, 64),
			seconds,
			reasons,
			amounts[2],
			amounts[3],
			rows,
			gaps,
			complete,
			strconv.FormatInt(row.Usage.SecondsUnavailableRows, 10),
			exportCsvCellGuard(strings.Join(row.CustomerModels, "; ")),
			grouping,
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
