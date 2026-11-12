package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

const (
	legacyUsageAnalyticsExportFieldVersion = 4
	upstreamUsageKnownSubtotalFieldVersion = 5
	usageAnalyticsExportFieldVersion       = 6
	usageAnalyticsExportTypeSelf           = "self"
	usageAnalyticsExportTypeCustomer       = "customer"
	usageAnalyticsExportTypeUpstream       = "upstream"
	usageAnalyticsRowTypeDaily             = "daily"
	usageAnalyticsRowTypePeriodTotal       = "period_total"
)

type UsageAnalyticsExportRequest struct {
	ReportDays []model.UsageReportDayReference
	Period     string
	Date       string
	View       string
	UserId     *int
	TokenId    *int
	Language   string
	Search     string
	ModelName  string
	Upstream   model.UsageUpstreamScope
}

// usageAnalyticsViewForJob derives the frozen view from job identity columns.
func usageAnalyticsViewForJob(job *model.CustomerExportJob) string {
	if job.TargetUserId == 0 {
		return usageAnalyticsExportTypeUpstream
	}
	return usageAnalyticsExportTypeCustomer
}

// usageAnalyticsPeriodFromFilters rebuilds the frozen Shanghai calendar period.
func usageAnalyticsPeriodFromFilters(filters model.CustomerExportFilters) (model.UsageAnalyticsPeriod, error) {
	start := time.Unix(filters.StartTimestamp, 0).In(model.UsageAnalyticsZone())
	date := start.Format("2006-01-02")
	var period string
	switch filters.EndTimestamp - filters.StartTimestamp {
	case 86400:
		period = model.UsageAnalyticsPeriodDay
	case 7 * 86400:
		period = model.UsageAnalyticsPeriodWeek
	case 28 * 86400, 29 * 86400, 30 * 86400, 31 * 86400:
		period = model.UsageAnalyticsPeriodMonth
	default:
		return model.UsageAnalyticsPeriod{}, errors.New("usage export range must be one day, week or month")
	}
	resolved, err := model.ResolveUsageAnalyticsPeriod(period, date, common.GetTimestamp())
	if err == nil && (resolved.StartTimestamp != filters.StartTimestamp || resolved.EndTimestamp != filters.EndTimestamp) {
		return model.UsageAnalyticsPeriod{}, errors.New("usage export range must align with Shanghai calendar boundaries")
	}
	return resolved, err
}

// SubmitUsageAnalyticsExport validates and freezes the submission scope, then
// accepts the job through the shared export queue. ClickHouse log backends are
// rejected like the other source-reading exports.
func SubmitUsageAnalyticsExport(actorId int, request UsageAnalyticsExportRequest) (*model.CustomerExportJob, error) {
	view := strings.TrimSpace(request.View)
	if view == "" {
		view = usageAnalyticsExportTypeSelf
	}
	if view != usageAnalyticsExportTypeSelf && view != usageAnalyticsExportTypeCustomer && view != usageAnalyticsExportTypeUpstream && view != "customers" {
		return nil, fmt.Errorf("%w: invalid view", ErrCustomerExportInvalidRequest)
	}
	if len(request.ModelName) > 255 || (request.ModelName != "" && view != usageAnalyticsExportTypeSelf && view != usageAnalyticsExportTypeCustomer) {
		return nil, fmt.Errorf("%w: %w", ErrCustomerExportInvalidRequest, common.NewMessage("Model filters require a customer usage view and a model name of at most 255 bytes."))
	}
	if request.TokenId != nil && (view != usageAnalyticsExportTypeSelf || *request.TokenId < 0) {
		return nil, fmt.Errorf("%w: invalid API key filter", ErrCustomerExportInvalidRequest)
	}
	if !request.Upstream.Valid() || (view != usageAnalyticsExportTypeUpstream && (request.Upstream.URLKey != "" || request.Upstream.ChannelID != nil)) {
		return nil, fmt.Errorf("%w: %w", ErrCustomerExportInvalidRequest, common.NewMessage("Invalid upstream usage scope"))
	}
	period, err := model.ResolveUsageAnalyticsPeriod(request.Period, request.Date, common.GetTimestamp())
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err.Error())
	}
	targetUserId, err := resolveUsageAnalyticsTarget(actorId, view, request.UserId)
	if err != nil {
		return nil, err
	}
	if err := model.AuthorizeUsageAnalyticsExport(context.Background(), actorId, targetUserId, view); err != nil {
		return nil, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	store, err := currentExportObjectStore()
	if err != nil {
		return nil, err
	}
	filters := model.CustomerExportFilters{
		UsageView: view, UsageSearch: strings.TrimSpace(request.Search),
		FieldVersion:   usageAnalyticsExportFieldVersion,
		ModelName:      request.ModelName,
		TokenId:        request.TokenId,
		QuotaPerUnit:   common.QuotaPerUnit,
		Currency:       operation_setting.GetQuotaDisplayType(),
		CurrencyRate:   operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate),
		StartTimestamp: period.StartTimestamp,
		EndTimestamp:   period.EndTimestamp,
		Timezone:       model.UsageAnalyticsTimezone,
		Language:       usageAnalyticsExportLanguage(request.Language),
	}
	if view == usageAnalyticsExportTypeUpstream {
		filters.UsageURLKey = request.Upstream.URLKey
		filters.ChannelId = request.Upstream.ChannelID
	}
	filters.UsageReportDays, err = model.FreezeUsageReportReferences(context.Background(), period, request.ReportDays)
	if err != nil {
		return nil, err
	}
	sourceVersion, err := adminUsageExportSourceVersion(context.Background(), targetUserId, filters)
	if err != nil {
		return nil, err
	}
	job, created, err := model.CreateCustomerExportJob(actorId, targetUserId, model.CustomerExportJobTypeUsageSummary, filters,
		model.CustomerExportReuse{StoreIdentity: store.ExportIdentity(), SourceVersion: sourceVersion})
	if err != nil {
		return nil, err
	}
	if created {
		if _, _, err := EnqueueSystemTask(SystemTaskTypeCustomerExport, nil); err != nil {
			common.SysLog("usage analytics export wake failed: " + err.Error())
		}
	}
	return job, nil
}

// resolveUsageAnalyticsTarget maps the requested view to the job target user.
// Admin authority is re-checked by AuthorizeCustomerExportJob on every read.
func resolveUsageAnalyticsTarget(actorId int, view string, requestedUser *int) (int, error) {
	switch view {
	case usageAnalyticsExportTypeSelf:
		return actorId, nil
	case usageAnalyticsExportTypeCustomer:
		if requestedUser == nil || *requestedUser <= 0 {
			return 0, fmt.Errorf("%w: customer view requires user_id", ErrCustomerExportInvalidRequest)
		}
		return *requestedUser, nil
	default:
		return 0, nil
	}
}

// usageAnalyticsExportLanguage restricts export language to en/zh (hard
// constraint: only these two locales are maintained).
func usageAnalyticsExportLanguage(language string) string {
	normalized := normalizeCustomerExportLanguage(language)
	if normalized != "en" && normalized != "zh" {
		return "en"
	}
	return normalized
}

// executeUsageAnalyticsExport runs the frozen aggregation once and writes the
// CSV through the shared writer, so page and export share one code path.
func executeUsageAnalyticsExport(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string) (*model.CustomerExportArtifact, error) {
	period, err := usageAnalyticsPeriodFromFilters(filters)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCustomerExportInvalidRequest, err.Error())
	}
	view := filters.UsageView
	if view == "" {
		view = usageAnalyticsViewForJob(job)
	}
	scope := customerExportScopeColumns{
		Language:     filters.Language,
		JobID:        job.JobID,
		ExportType:   job.JobType,
		PeriodStart:  filters.StartTimestamp,
		PeriodEnd:    filters.EndTimestamp - 1,
		QuotaPerUnit: filters.QuotaPerUnit,
		Currency:     filters.Currency,
		CurrencyRate: filters.CurrencyRate,
		Timezone:     filters.Timezone,
		CustomerId:   job.TargetUserId,
		GeneratedAt:  common.GetTimestamp(),
	}
	var sourceVersion string
	if scope.CustomerId > 0 {
		name, nameErr := usageExportCustomerName(scope.CustomerId)
		scope.CustomerName = name
		if nameErr == nil && view == usageAnalyticsExportTypeCustomer {
			sourceVersion = usageExportIdentityVersion([]model.UsageCustomerOverviewRow{{UserId: scope.CustomerId, Username: name}})
		}
	}
	writer, err := newCustomerExportCsvWriter(workDir, "usage", customerExportCsvShardBytes)
	if err != nil {
		return nil, err
	}
	defer writer.Cleanup()
	writer.header = usageAnalyticsExportHeader(filters.Language)
	if view == usageAnalyticsExportTypeUpstream && filters.FieldVersion >= upstreamUsageKnownSubtotalFieldVersion {
		if filters.Language == "zh" {
			writer.header = append(writer.header, "折后参考金额已知小计")
		} else {
			writer.header = append(writer.header, "Known subtotal of reference amount")
		}
	}
	rows, overviewVersion, err := usageAnalyticsExportRows(ctx, job, view, period, filters, scope)
	if err != nil {
		return nil, err
	}
	if overviewVersion != "" {
		sourceVersion = overviewVersion
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := writer.AppendRecord(row); err != nil {
			return nil, err
		}
	}
	files, paths, lines, _, err := writer.Finish()
	if errors.Is(err, errCustomerExportNoFiles) {
		artifact := &model.CustomerExportArtifact{Files: []model.CustomerExportArtifactFile{}, GeneratedAt: common.GetTimestamp(), SourceVersion: sourceVersion}
		if view == usageAnalyticsExportTypeSelf || sourceVersion != "" {
			store, storeErr := currentExportObjectStore()
			if storeErr != nil {
				return nil, storeErr
			}
			artifact.StoreIdentity = store.ExportIdentity()
		}
		return artifact, nil
	}
	if err != nil {
		return nil, err
	}
	artifact, err := uploadCustomerExportArtifact(ctx, job.JobID, files, paths, lines)
	if err == nil {
		artifact.GeneratedAt = scope.GeneratedAt
		artifact.SourceVersion = sourceVersion
	}
	return artifact, err
}

// usageAnalyticsExportHeader renders CSV headers per language (en/zh only).
func usageAnalyticsExportHeader(language string) []string {
	if language == "zh" {
		return []string{"行类型", "日期", "客户 ID", "客户", "API Key ID", "API Key", "客户模型", "URL 归组", "渠道 ID", "渠道", "上游模型", "上游模型未记录", "计费方式", "调用次数", "成功", "失败", "取消", "其他终态", "输入 Token", "输出 Token", "缓存读取", "缓存写入", "图片张数", "秒数", "扣减 quota", "退款 quota", "净额 quota", "估算原价金额", "折后参考金额", "未关联退款", "缺用量行数", "缺金额行数", "金额待更新行数", "无金额用量行数", "已计价测试行数", "渠道折扣", "估算原因", "输入图片 Token", "输出图片 Token", "输入音频 Token", "输出音频 Token", "币种", "单位 quota", "汇率", "任务 ID", "生成时间", "周期开始", "周期结束", "时区", "客户周期折扣构成"}
	}
	return []string{"Row type", "Date", "Customer ID", "Customer", "API Key ID", "API Key", "Customer model", "URL grouping", "Channel ID", "Channel", "Provider model", "Provider model fallback", "Billing mode", "Calls", "Success", "Failure", "Cancelled", "Other", "Input tokens", "Output tokens", "Cache read", "Cache write", "Images", "Seconds", "Gross quota", "Refund quota", "Net quota", "Original amount (estimated)", "Reference amount (after discount)", "Unlinked refund", "Missing-token rows", "Missing-money rows", "Money-pending rows", "Usage-only rows", "Test-priced rows", "Channel discounts", "Estimate reasons", "Image input tokens", "Image output tokens", "Audio input tokens", "Audio output tokens", "Currency", "Quota per unit", "Currency rate", "Job ID", "Generated at", "Period start", "Period end", "Time zone", "Customer period discount combinations"}
}

// usageAnalyticsScopeTail renders the trailing scope cells shared by all rows.
func usageAnalyticsScopeTail(scope customerExportScopeColumns) []string {
	return []string{
		scope.Currency,
		strconv.FormatFloat(scope.QuotaPerUnit, 'f', -1, 64),
		strconv.FormatFloat(scope.CurrencyRate, 'f', -1, 64),
	}
}

// usageAnalyticsMetricsRow renders one metrics block as CSV cells; the
// channel-discount summary sits between the usage-only counter and the
// estimate reasons, matching the header order exactly.
func usageAnalyticsMetricsRow(metrics model.UsageAnalyticsMetrics, discount string, scope customerExportScopeColumns, fieldVersion int) []string {
	cells := []string{
		strconv.FormatInt(metrics.TotalCalls, 10),
		strconv.FormatInt(metrics.SuccessCalls, 10),
		strconv.FormatInt(metrics.FailureCalls, 10),
		strconv.FormatInt(metrics.CancelledCalls, 10),
		strconv.FormatInt(metrics.OtherResultCalls, 10),
		strconv.FormatInt(metrics.InputTokens, 10),
		strconv.FormatInt(metrics.OutputTokens, 10),
		strconv.FormatInt(metrics.CacheReadTokens, 10),
		strconv.FormatInt(metrics.CacheWriteTokens, 10),
		strconv.FormatInt(metrics.ImageCount, 10),
		usageAnalyticsFormatSeconds(metrics.Seconds),
		strconv.FormatInt(metrics.GrossQuota, 10),
		strconv.FormatInt(metrics.RefundQuota, 10),
		strconv.FormatInt(metrics.NetQuota, 10),
		usageAnalyticsExportAmount(metrics.OriginalQuotaEstimate, scope),
		usageAnalyticsExportAmount(metrics.ReferenceAmount, scope),
		strconv.FormatInt(metrics.UnlinkedRefundQuota, 10),
		strconv.FormatInt(metrics.RowsMissingTokens, 10),
		strconv.FormatInt(metrics.RowsMissingMoney, 10),
		strconv.FormatInt(metrics.RowsMoneyPending, 10),
		strconv.FormatInt(metrics.UsageOnlyRows, 10),
		strconv.FormatInt(metrics.TestPricedRows, 10),
		exportCsvCellGuard(discount),
		strings.Join(metrics.EstimateReasons, ";"),
	}
	if metrics.RowsMissingTokens > 0 {
		for _, i := range []int{5, 6, 7, 8} {
			if cells[i] == "0" {
				cells[i] = ""
			}
		}
	}
	if metrics.RowsMissingMoney > 0 {
		cells[11], cells[12] = "", ""
	}
	if metrics.RowsMoneyPending > 0 && metrics.NetQuota == 0 {
		cells[13] = ""
	}
	if fieldVersion >= usageAnalyticsExportFieldVersion {
		if !metrics.ImageCountKnown && metrics.ImageCount == 0 {
			cells[9] = ""
		}
		if metrics.ImageCountMissingRows > 0 {
			cells[23] = strings.Trim(cells[23]+";images_unrecorded:"+strconv.FormatInt(metrics.ImageCountMissingRows, 10), ";")
		}
	}
	if metrics.SecondsMissingRows > 0 {
		cells[23] = strings.Trim(cells[23]+";seconds_unrecorded:"+strconv.FormatInt(metrics.SecondsMissingRows, 10), ";")
	}
	for _, key := range []string{"image_input", "image_output", "audio_input", "audio_output"} {
		value := ""
		if count, known := metrics.TokenDetails[key]; known {
			value = strconv.FormatInt(count, 10)
		}
		cells = append(cells, value)
	}
	return cells
}

// usageAnalyticsFormatSeconds renders recorded seconds; missing stays blank.
func usageAnalyticsFormatSeconds(seconds *decimal.Decimal) string {
	if seconds == nil {
		return ""
	}
	return seconds.String()
}

// usageAnalyticsExportAmount converts a settled quota figure for display.
func usageAnalyticsExportAmount(value *int64, scope customerExportScopeColumns) string {
	if value == nil {
		return ""
	}
	return exportCurrencyAmount(strconv.FormatInt(*value, 10), scope)
}

// usageAnalyticsGroupCells renders the group identity cells of one row; empty
// cells mean "not applicable for this row type" and keep the schema fixed.
func usageAnalyticsGroupCells(values ...string) []string {
	cells := make([]string, 0, 12)
	for _, value := range values {
		cells = append(cells, exportCsvCellGuard(value))
	}
	return cells
}

// usageAnalyticsExportRows runs the frozen aggregation once and emits grand
// total, per-group daily and per-group period-total rows.
func usageAnalyticsExportRows(ctx context.Context, job *model.CustomerExportJob, view string, period model.UsageAnalyticsPeriod, filters model.CustomerExportFilters, scope customerExportScopeColumns) ([][]string, string, error) {
	records := make([][]string, 0, 64)
	if len(filters.UsageReportDays) == 0 {
		return nil, "", errCustomerExportFormatChanged
	}
	ctx = model.WithUsageReportReferences(ctx, filters.UsageReportDays)
	publishedDates := make(map[string]bool)
	for _, ref := range filters.UsageReportDays {
		publishedDates[time.Unix(ref.Start, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)] = true
	}
	var rowDiscount string
	var customerDiscounts string
	appendRow := func(rowType string, date string, groupCells []string, metrics model.UsageAnalyticsMetrics) {
		record := make([]string, 0, 48)
		record = append(record, rowType, date)
		record = append(record, groupCells...)
		cells := usageAnalyticsMetricsRow(metrics, rowDiscount, scope, filters.FieldVersion)
		if rowType == usageAnalyticsRowTypeDaily {
			for _, day := range period.Days {
				if day.Date == date && (day.Future || !publishedDates[date]) {
					rowType = "not_started"
					if !day.Future {
						rowType = "not_published"
					}
					record[0] = rowType
					for i := range cells {
						cells[i] = ""
					}
				}
			}
		}
		record = append(record, cells...)
		record = append(record, usageAnalyticsScopeTail(scope)...)
		record = append(record, job.JobID, formatExportTimestamp(scope.GeneratedAt),
			formatExportTimestamp(filters.StartTimestamp), formatExportTimestamp(filters.EndTimestamp), filters.Timezone, customerDiscounts)
		if view == usageAnalyticsExportTypeUpstream && filters.FieldVersion >= upstreamUsageKnownSubtotalFieldVersion {
			amount := ""
			if rowType != "not_started" && rowType != "not_published" {
				amount = usageAnalyticsExportAmount(metrics.KnownReferenceAmount, scope)
			}
			record = append(record, amount)
		}
		records = append(records, record)
	}
	if view == "customers" {
		overview, _, err := model.GetPublishedUsageOverview(ctx, period, filters.UsageSearch)
		if err != nil {
			return nil, "", err
		}
		for _, customer := range overview.Customers {
			cells := usageAnalyticsGroupCells(strconv.Itoa(customer.UserId), customer.Username, "", "", "", "", "", "", "", "", "")
			for i, metrics := range customer.Days {
				appendRow(usageAnalyticsRowTypeDaily, period.Days[i].Date, cells, metrics)
			}
			appendRow(usageAnalyticsRowTypePeriodTotal, "", cells, customer.Total)
		}
		appendRow("grand_total", "", make([]string, 11), overview.Total)
		return records, usageExportIdentityVersion(overview.Customers), nil
	}
	if view == usageAnalyticsExportTypeUpstream {
		upstreamView, _, err := model.GetPublishedUsageUpstream(ctx, period, model.UsageUpstreamScope{URLKey: filters.UsageURLKey, ChannelID: filters.ChannelId})
		if err != nil {
			return nil, "", err
		}
		for _, group := range upstreamView.UrlGroups {
			for _, modelGroup := range group.Models {
				for _, channel := range modelGroup.Channels {
					discounts := strings.Join(usageAnalyticsDiscountCells(channel), ";")
					modelRow := modelGroup
					modelRow.Days, modelRow.Total = channel.Days, channel.Total
					for dayIdx, dayMetrics := range modelRow.Days {

						cells := usageAnalyticsGroupCells("", "", "", "", "", group.UrlKey, strconv.Itoa(channel.ChannelId), channel.ChannelName, modelRow.ProviderModel, exportYesNo(modelRow.ProviderModelFallback), modelRow.BillingMode)
						rowDiscount = discounts
						if filters.FieldVersion >= upstreamUsageKnownSubtotalFieldVersion {
							rowDiscount = strings.Join(dayMetrics.ReferenceCoefficients, ";")
						}
						appendRow(usageAnalyticsRowTypeDaily, period.Days[dayIdx].Date, cells, dayMetrics)
						rowDiscount = ""
					}
					cells := usageAnalyticsGroupCells("", "", "", "", "", group.UrlKey, strconv.Itoa(channel.ChannelId), channel.ChannelName, modelRow.ProviderModel, exportYesNo(modelRow.ProviderModelFallback), modelRow.BillingMode)
					rowDiscount = discounts
					if filters.FieldVersion >= upstreamUsageKnownSubtotalFieldVersion {
						rowDiscount = strings.Join(modelRow.Total.ReferenceCoefficients, ";")
					}
					appendRow(usageAnalyticsRowTypePeriodTotal, "", cells, modelRow.Total)
					rowDiscount = ""
				}
			}
		}
		var channelID, channelName string
		if filters.ChannelId != nil {
			channelID = strconv.Itoa(*filters.ChannelId)
			if len(upstreamView.Channels) == 1 {
				channelName = upstreamView.Channels[0].ChannelName
			}
		}
		appendRow("grand_total", "", usageAnalyticsGroupCells("", "", "", "", "", filters.UsageURLKey, channelID, channelName, "", "", ""), upstreamView.Total)
		return records, "", nil
	}
	target := job.TargetUserId
	customerView, _, err := model.GetPublishedUsageCustomer(ctx, period, target, filters.ModelName)
	if err != nil {
		return nil, "", err
	}
	if filters.TokenId != nil {
		index := slices.IndexFunc(customerView.Keys, func(key model.UsageCustomerKeyGroup) bool {
			return key.TokenId == *filters.TokenId
		})
		if index < 0 {
			return records, "", nil
		}
		customerView.Total = customerView.Keys[index].Total
		customerView.Keys = customerView.Keys[index : index+1]
	}
	if len(customerView.Keys) == 0 && filters.ModelName != "" {
		return records, "", nil
	}
	for _, key := range customerView.Keys {
		if key.TokenId == 0 {
			key.TokenName = "No API Key"
			if filters.Language == "zh" {
				key.TokenName = "无 API Key"
			}
		}
		for _, modelRow := range key.Models {
			for dayIdx, dayMetrics := range modelRow.Days {

				cells := usageAnalyticsGroupCells(strconv.Itoa(target), scope.CustomerName, strconv.Itoa(key.TokenId), key.TokenName, modelRow.ModelName, "", "", "", "", "", "")
				appendRow(usageAnalyticsRowTypeDaily, period.Days[dayIdx].Date, cells, dayMetrics)
			}
			cells := usageAnalyticsGroupCells(strconv.Itoa(target), scope.CustomerName, strconv.Itoa(key.TokenId), key.TokenName, modelRow.ModelName, "", "", "", "", "", "")
			customerDiscounts = usageCustomerDiscountExport(modelRow, filters.Language, scope)
			appendRow(usageAnalyticsRowTypePeriodTotal, "", cells, modelRow.Total)
			customerDiscounts = ""
		}
	}
	appendRow("grand_total", "", make([]string, 11), customerView.Total)
	return records, "", nil
}

// usageAnalyticsDiscountCells renders the channel coefficient summary cell.
func usageAnalyticsDiscountCells(channel model.UsageUpstreamChannelRow) []string {
	if len(channel.Discounts) == 0 {
		return []string{""}
	}
	values := make([]string, 0, len(channel.Discounts))
	for _, discount := range channel.Discounts {
		zone := model.UsageAnalyticsZone()
		start := time.Unix(discount.PeriodStart, 0).In(zone)
		periodValue := start.Format("2006-01") + ":" + discount.Value.String()
		if len(discount.Changes) > 0 {
			segments := []string{start.Format(time.DateOnly) + ":" + discount.Value.String()}
			for _, change := range discount.Changes {
				segments = append(segments, time.Unix(change.EffectiveFrom, 0).In(zone).Format(time.DateOnly)+":"+change.Value.String())
			}
			periodValue = strings.Join(segments, " / ")
		}
		values = append(values, periodValue+" ("+discount.Source+", v"+strconv.FormatInt(discount.Version, 10)+")")
	}
	return values
}

// Regeneration preserves the accepted calendar, currency and discount evidence.
func ResubmitUsageAnalyticsExport(actorID int, sourceJobID string, selfOnly bool) (*model.CustomerExportJob, error) {
	source, err := model.GetCustomerExportJobForOwner(sourceJobID, actorID)
	if err != nil {
		return nil, err
	}
	if source.JobType != model.CustomerExportJobTypeUsageSummary || (selfOnly && source.TargetUserId != actorID) {
		return nil, model.ErrCustomerExportNotFound
	}
	if err := model.AuthorizeCustomerExportJob(context.Background(), source); err != nil {
		return nil, err
	}
	filters, err := source.DecodeFilters()
	if err != nil {
		return nil, err
	}
	if _, err = usageAnalyticsPeriodFromFilters(filters); err != nil {
		return nil, err
	}
	if len(filters.UsageReportDays) == 0 {
		return nil, errCustomerExportFormatChanged
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err = currentExportObjectStore(); err != nil {
		return nil, err
	}
	filters.FieldVersion = usageAnalyticsExportFieldVersion
	if filters.UsageView == usageAnalyticsExportTypeUpstream || (filters.UsageView == "" && usageAnalyticsViewForJob(source) == usageAnalyticsExportTypeUpstream) {
		filters.UsageView = usageAnalyticsExportTypeUpstream
	}
	job, created, err := model.CreateCustomerExportJob(actorID, source.TargetUserId, source.JobType, filters)
	if err != nil {
		return nil, err
	}
	if created {
		_, _, _ = EnqueueSystemTask(SystemTaskTypeCustomerExport, nil)
	}
	return job, nil
}
