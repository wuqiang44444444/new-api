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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

const (
	usageAnalyticsExportFieldVersion = 11
	usageAnalyticsExportTypeSelf     = "self"
	usageAnalyticsExportTypeCustomer = "customer"
	usageAnalyticsExportTypeUpstream = "upstream"
	usageAnalyticsRowTypeDaily       = "daily"
	usageAnalyticsRowTypePeriodTotal = "period_total"
)

type UsageAnalyticsExportRequest struct {
	ReportDays []model.UsageReportDayReference
	Period     string
	Date       string
	View       string
	Grain      string
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
	if request.Grain != "" && request.Grain != "daily" && request.Grain != "period" {
		return nil, fmt.Errorf("%w: invalid usage export grain", ErrCustomerExportInvalidRequest)
	}
	if request.Grain == "daily" {
		request.Grain = ""
	}
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
		UsageView: view, UsageExportGrain: request.Grain, UsageSearch: strings.TrimSpace(request.Search),
		FieldVersion:   usageAnalyticsExportFieldVersion,
		ModelName:      request.ModelName,
		TokenId:        request.TokenId,
		QuotaPerUnit:   common.QuotaPerUnit,
		Currency:       operation_setting.GetQuotaDisplayType(),
		CurrencyRate:   operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate),
		StartTimestamp: period.StartTimestamp,
		EndTimestamp:   period.EndTimestamp,
		Timezone:       model.UsageAnalyticsTimezone,
		Language:       billingExportLanguage,
	}
	if view == usageAnalyticsExportTypeUpstream {
		filters.UsageURLKey = request.Upstream.URLKey
		filters.ChannelId = request.Upstream.ChannelID
	}
	snapshot, err := model.FreezeUsageReportExport(context.Background(), period, request.ReportDays)
	if err != nil {
		return nil, err
	}
	filters.UsageReportDays = snapshot.References
	filters.UsageIncompleteDates = snapshot.IncompleteDays
	expected := 0
	for _, day := range period.Days {
		if !day.Future {
			expected++
		}
	}
	filters.UsageExpectedDays = &expected
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
	writer.header = csvLabels(filters.Language, scope.Currency, usageCSVColumns(view, filters.UsageExportGrain))
	if filters.UsageExportGrain == "period" {
		writer.prefix += "-period"
	} else {
		writer.prefix += "-daily"
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

// usageAnalyticsExportRows emits exactly one grain and one row per leaf.
func usageAnalyticsExportRows(ctx context.Context, job *model.CustomerExportJob, view string, period model.UsageAnalyticsPeriod, filters model.CustomerExportFilters, scope customerExportScopeColumns) ([][]string, string, error) {
	if len(filters.UsageReportDays) == 0 {
		return nil, "", errCustomerExportFormatChanged
	}
	ctx = model.WithUsageReportReferences(ctx, filters.UsageReportDays)
	publishedDates := make(map[string]bool, len(filters.UsageReportDays))
	for _, ref := range filters.UsageReportDays {
		publishedDates[time.Unix(ref.Start, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)] = true
	}
	columns := usageCSVColumns(view, filters.UsageExportGrain)
	records := make([][]string, 0, 64)
	upstream := view == usageAnalyticsExportTypeUpstream
	expected := 0
	if filters.UsageExpectedDays != nil {
		expected = *filters.UsageExpectedDays
	} else {
		for _, day := range period.Days {
			if !day.Future {
				expected++
			}
		}
	}
	coverageNote := csvReportCoverage(scope.Language, &model.BillingExportCoverage{
		PublishedDays: len(publishedDates), ExpectedDays: expected, IncompleteDays: len(filters.UsageIncompleteDates),
	})
	if len(filters.UsageIncompleteDates) > 0 {
		coverageNote = csvNotes(coverageNote, csvText(scope.Language, "Incomplete dates ", "覆盖不完整日期 ")+strings.Join(filters.UsageIncompleteDates, ", "))
	}
	appendLeaf := func(identities map[string]string, days []model.UsageAnalyticsMetrics, total model.UsageAnalyticsMetrics, identityNote string) {
		appendRow := func(date string, metrics model.UsageAnalyticsMetrics, missing string) {
			values := make(map[string]string)
			if missing == "" {
				values = usageCSVMetricValues(metrics, scope, upstream)
				values["discount"] = strings.Join(metrics.ReferenceCoefficients, " / ")
			}
			for key, value := range identities {
				values[key] = exportCsvCellGuard(value)
			}
			values["date"] = date
			values["notes"] = csvNotes(missing, values["notes"], identityNote, coverageNote)
			records = append(records, csvProjection(columns, values))
		}
		if filters.UsageExportGrain == "period" {
			start := time.Unix(filters.StartTimestamp, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)
			end := time.Unix(filters.EndTimestamp-1, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)
			appendRow(start+" — "+end, total, "")
			return
		}
		for i, day := range period.Days {
			if i >= expected {
				break
			}
			if !publishedDates[day.Date] {
				appendRow(day.Date, model.UsageAnalyticsMetrics{}, csvText(scope.Language, "Not published", "日报未发布"))
			} else if i < len(days) {
				appendRow(day.Date, days[i], "")
			}
		}
	}
	if view == "customers" {
		overview, _, err := model.GetPublishedUsageOverview(ctx, period, filters.UsageSearch)
		if err != nil {
			return nil, "", err
		}
		for _, customer := range overview.Customers {
			appendLeaf(map[string]string{"customer": customer.Username}, customer.Days, customer.Total, "")
		}
		return records, usageExportIdentityVersion(overview.Customers), nil
	}
	if upstream {
		upstreamView, _, err := model.GetPublishedUsageUpstream(ctx, period, model.UsageUpstreamScope{URLKey: filters.UsageURLKey, ChannelID: filters.ChannelId})
		if err != nil {
			return nil, "", err
		}
		for _, group := range upstreamView.UrlGroups {
			for _, modelGroup := range group.Models {
				var identityNote string
				if modelGroup.ProviderModelFallback {
					identityNote = csvText(scope.Language, "Showing the customer model because the upstream model was not recorded", "上游模型未记录，显示客户模型")
				}
				for _, channel := range modelGroup.Channels {
					appendLeaf(map[string]string{
						"url": group.UrlKey, "channel_id": strconv.Itoa(channel.ChannelId), "channel": channel.ChannelName,
						"model": modelGroup.ProviderModel, "mode": customerStatementExportValue(scope.Language, "billing_mode", modelGroup.BillingMode),
					}, channel.Days, channel.Total, identityNote)
				}
			}
		}
		return records, "", nil
	}
	customerView, _, err := model.GetPublishedUsageCustomerByBillingMode(ctx, period, job.TargetUserId, filters.ModelName)
	if err != nil {
		return nil, "", err
	}
	for _, key := range customerView.Keys {
		if filters.TokenId != nil && key.TokenId != *filters.TokenId {
			continue
		}
		token := key.TokenName
		if key.TokenId == 0 {
			token = csvText(scope.Language, "No API Key", "无 API Key")
		}
		for _, modelRow := range key.Models {
			appendLeaf(map[string]string{"customer": scope.CustomerName, "token": token, "model": modelRow.ModelName,
				"mode": customerStatementExportValue(scope.Language, "billing_mode", modelRow.BillingMode)}, modelRow.Days, modelRow.Total, "")
		}
	}
	return records, "", nil
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
	period, err := usageAnalyticsPeriodFromFilters(filters)
	if err != nil {
		return nil, err
	}
	if len(filters.UsageReportDays) == 0 {
		return nil, errCustomerExportFormatChanged
	}
	if _, err = model.FreezeUsageReportReferences(context.Background(), period, filters.UsageReportDays); err != nil {
		return nil, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	if _, err = currentExportObjectStore(); err != nil {
		return nil, err
	}
	filters.FieldVersion = usageAnalyticsExportFieldVersion
	filters.Language = billingExportLanguage
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
