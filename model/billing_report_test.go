package model

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingReportCoefficientAndAuditCommitTogether(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	const channelID = 23
	require.NoError(t, db.Create(&Channel{Id: channelID, Name: "channel"}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, ChannelId: channelID, ModelName: "test", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}).Error)
	require.NoError(t, BuildBillingReportDay(context.Background(), start, "billing-test", "runner", nil, nil))

	simulated := errors.New("discount audit unavailable")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject-discount-audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "provider_billing_audits" {
			tx.AddError(simulated)
		}
	}))
	discount := ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: channelID, Discount: decimal.RequireFromString("0.5")}
	require.ErrorIs(t, SaveProviderChannelBillingDiscount(&discount, 0, 7), simulated)
	require.NoError(t, db.Callback().Create().Remove("test:reject-discount-audit"))
	for _, model := range []any{&ProviderChannelBillingDiscount{}, &ProviderBillingAudit{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		assert.Zero(t, count)
	}
	discount.Id = 0
	require.NoError(t, SaveProviderChannelBillingDiscount(&discount, 0, 7))
	var count int64
	require.NoError(t, db.Model(&ProviderBillingAudit{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&billingReportRebuild{}).Count(&count).Error)
	assert.Zero(t, count, "a discount update does not rebuild published usage")
}

func setupBillingReportTest(t *testing.T) (*gorm.DB, int64, int64) {
	t.Helper()
	var db *gorm.DB
	if os.Getenv("TEST_BILLING_REPORT_DSN") == "" {
		db = setupUsageAnalyticsTestDB(t)
	} else {
		// Optional integration fixture. The DSN must point to a dedicated empty DB.
		priorDB, priorLog := DB, LOG_DB
		priorMainType, priorLogType := common.MainDatabaseType(), common.LogDatabaseType()
		testDB, kind, err := chooseDB("TEST_BILLING_REPORT_DSN", false)
		require.NoError(t, err)
		sqlDB, err := testDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() {
			DB, LOG_DB = priorDB, priorLog
			common.SetDatabaseTypes(priorMainType, priorLogType)
			_ = sqlDB.Close()
		})
		DB, LOG_DB, db = testDB, testDB, testDB
		common.SetDatabaseTypes(kind, kind)
		require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &Log{}, &Task{}, &TaskBillingDelivery{}))
		require.NoError(t, migrateBillingReconciliationDB())
	}
	require.NoError(t, MigrateDailyReports(db))
	require.NoError(t, db.AutoMigrate(&SystemTask{}, &SystemTaskLock{}))
	now := time.Now().In(UsageAnalyticsZone()).AddDate(0, -2, 0)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	require.NoError(t, db.Create(&SystemTaskLock{Type: UsageReportTaskType, TaskID: "billing-test", LockedBy: "runner", LockedUntil: common.GetTimestamp() + 60}).Error)
	require.NoError(t, db.Create(&User{Id: 7, Username: "billing-customer"}).Error)
	return db, month.Unix(), month.AddDate(0, 1, 0).Unix() - 1
}

func TestPublishedBillingReportPinsSummaryDetailsAndExportAfterSourceChanges(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	url := "https://provider.example/v1"
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "account A", BaseURL: &url}).Error)
	other := `{"task_id":"video-1","is_task":true,"group_ratio":0.8,"contract_applicable":false,"upstream_model_name":"provider-video","model_price":1}`
	chargeFacts := `{"task_id":"video-1","is_task":true,"group_ratio":0.8,"contract_applicable":false,"upstream_model_name":"provider-video","model_price":1,"admin_info":{"statement_original_quota":127}}`
	refundFacts := `{"task_id":"video-1","is_task":true,"group_ratio":0.8,"contract_applicable":false,"upstream_model_name":"provider-video","model_price":1,"admin_info":{"statement_original_quota":39}}`
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 11, TokenName: "key", ModelName: "video", ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 20, Quota: 100, RequestId: "charge", Other: chargeFacts},
		{UserId: 7, TokenId: 11, TokenName: "key", ModelName: "video", ChannelId: 21, Type: LogTypeRefund, CreatedAt: start + 30, Quota: 30, RequestId: "same-day", Other: refundFacts},
		{UserId: 7, TokenId: 11, TokenName: "key", ModelName: "video", ChannelId: 21, Type: LogTypeRefund, CreatedAt: start + 2*billingReportDaySeconds + 30, Quota: 10, RequestId: "later", Other: other},
		{UserId: 99, TokenId: 13, TokenName: "separate", ModelName: "video", ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 30, Quota: 9, RequestId: "different-user", Other: other},
		{UserId: 7, TokenName: "Model test", Content: "Model test", ModelName: "video", ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 40, Quota: 999, RequestId: "test", Other: other},
	}).Error)
	expected, err := GetBillingCustomerStatement(ctx, 7, start, end, "api_key", 0, "", "", BillingStatementReadPolicy{RecordedAmountsOnly: true})
	require.NoError(t, err)
	expectedProvider, err := getProviderBillingURLSummary(ctx, start, end, start, "", 0)
	require.NoError(t, err)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildBillingReportDay(ctx, start+2*billingReportDaySeconds, "billing-test", "runner", nil, nil))
	pinned, report, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	require.Equal(t, 2, report.PublishedDays)
	require.Greater(t, report.ExpectedDays, report.PublishedDays)
	statement, err := GetBillingCustomerStatement(pinned, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	expectedJSON, err := common.Marshal(expected)
	require.NoError(t, err)
	actualJSON, err := common.Marshal(statement)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJSON), string(actualJSON))
	assert.EqualValues(t, 60, statement.Summary.NetQuota)
	assert.Nil(t, statement.OriginalQuota)
	assert.Nil(t, statement.DiscountQuota)

	firstDay, _, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	complete, err := GetBillingCustomerStatement(firstDay, 7, start, start+billingReportDaySeconds-1, "api_key", 0, "", "")
	require.NoError(t, err)
	require.NotNil(t, complete.OriginalQuota)
	require.NotNil(t, complete.DiscountQuota)
	assert.EqualValues(t, 88, *complete.OriginalQuota)
	assert.EqualValues(t, 18, *complete.DiscountQuota)
	require.Len(t, complete.DiscountCombinations, 1)
	require.NotNil(t, complete.DiscountCombinations[0].OriginalQuota)
	assert.EqualValues(t, 88, *complete.DiscountCombinations[0].OriginalQuota)

	list, err := GetBillingCustomerStatementList(pinned, start, end, "billing-customer", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.EqualValues(t, 60, list.Items[0].Usage.NetQuota)

	upstream, err := getProviderBillingURLSummary(pinned, start, end, start, "", 0)
	require.NoError(t, err)
	require.Len(t, upstream.Groups, 1)
	assert.Equal(t, expectedProvider.Groups[0].Usage, upstream.Groups[0].Usage)
	assert.Equal(t, expectedProvider.Groups[0].ReferenceAmount, upstream.Groups[0].ReferenceAmount)
	assert.Equal(t, expectedProvider.Groups[0].DataQuality, upstream.Groups[0].DataQuality)

	filter := BillingStatementLogFilter{UserId: 7, Start: start, End: end, GroupFunds: true}
	groups, err := GetBillingStatementLogs(pinned, filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, groups.FundGroups)
	require.Len(t, *groups.FundGroups, 2)
	assert.EqualValues(t, -10, (*groups.FundGroups)[0].NetQuota)
	assert.EqualValues(t, 70, (*groups.FundGroups)[1].NetQuota)
	filter.GroupFunds, filter.FundGroup = false, (*groups.FundGroups)[1].Key
	lines, err := GetBillingStatementLogs(pinned, filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, lines.Items, 2)
	assert.EqualValues(t, 70, lines.Quota)
	exports, _, err := ReadBillingReportCustomerExportBatch(pinned, 7, CustomerExportFilters{StartTimestamp: start, EndTimestamp: end + 1}, 0, 100)
	require.NoError(t, err)
	require.Len(t, exports, 3)
	assert.Equal(t, "charge", exports[0].RequestId)
	assert.Equal(t, "127", exports[0].RecordedOriginal)
	assert.Equal(t, "-39", exports[1].RecordedOriginal)
	assert.Empty(t, exports[2].RecordedOriginal)
	assert.Equal(t, "later", exports[2].RequestId)
	upstreamDetails, err := GetUpstreamBillingDetails(pinned, UpstreamBillingDetailFilter{Start: start, End: end, ChannelIds: []int{21}, URLKey: url}, 1, 20, false)
	require.NoError(t, err)
	assert.EqualValues(t, 5, upstreamDetails.Total)

	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, ModelName: "video", ChannelId: 21, Type: LogTypeRefund, CreatedAt: start + 2*billingReportDaySeconds + 40, Quota: 5, RequestId: "new-refund", Other: other}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start+2*billingReportDaySeconds, "billing-test", "runner", nil, nil))
	latest, newReport, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	assert.NotEqual(t, report.Version, newReport.Version)
	latestStatement, err := GetBillingCustomerStatement(latest, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 55, latestStatement.Summary.NetQuota)
	require.NoError(t, db.Where("created_at >= ?", start).Delete(&Log{}).Error)
	old, _, err := ResolveBillingReport(ctx, start, end, report.Version)
	require.NoError(t, err)
	statement, err = GetBillingCustomerStatement(old, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 60, statement.Summary.NetQuota)
	exports, _, err = ReadBillingReportCustomerExportBatch(old, 7, CustomerExportFilters{StartTimestamp: start, EndTimestamp: end + 1}, 0, 100)
	require.NoError(t, err)
	require.Len(t, exports, 3)
	require.Error(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	_, stillLatest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	assert.Equal(t, newReport.Version, stillLatest.Version)
}

func TestPublishedUpstreamDiscountChangesUseShanghaiDaysAndCanReplaceTheMonth(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	const channelID = 45
	url := "https://provider.example/v1"
	require.NoError(t, db.Create(&Channel{Id: channelID, Name: "provider", BaseURL: &url}).Error)
	boundary := start + billingReportDaySeconds
	facts := `{"group_ratio":1,"contract_applicable":false,"model_ratio":1}`
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ChannelId: channelID, ModelName: "model", Type: LogTypeConsume, CreatedAt: boundary - 1, Quota: 100, Other: facts},
		{UserId: 7, ChannelId: channelID, ModelName: "model", Type: LogTypeConsume, CreatedAt: boundary, Quota: 100, Other: facts},
	}).Error)
	for _, day := range []int64{start, boundary} {
		require.NoError(t, BuildBillingReportDay(ctx, day, "billing-test", "runner", nil, nil))
	}
	pinned, _, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	save := func(value string, expected, date int64) error {
		return SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{
			ChannelId: channelID, PeriodStart: start, Discount: decimal.RequireFromString(value),
		}, expected, 7, date)
	}
	require.NoError(t, save("0.9", 0, 0))
	require.Error(t, save("0.8", 1, boundary+1))
	require.NoError(t, save("0.8", 1, boundary))
	require.ErrorIs(t, save("0.7", 1, boundary), ErrBillingReconciliationVersionConflict)
	require.NoError(t, save("0.7", 2, boundary+billingReportDaySeconds))

	page, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{Level: "url_channels", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, page.URLChannels, 1)
	require.NotNil(t, page.URLChannels[0].Discount)
	require.Len(t, page.URLChannels[0].Discount.Changes, 2)
	require.NotNil(t, page.Totals.OriginalAmount)
	require.NotNil(t, page.Totals.ReferenceAmount)
	assert.EqualValues(t, 200, *page.Totals.OriginalAmount)
	assert.EqualValues(t, 170, *page.Totals.ReferenceAmount)
	models, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{
		Level: "url_models", ReportURL: url, ChannelID: channelID, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, models.ModelRows, 1)
	assert.Equal(t, page.Totals.ReferenceAmount, models.ModelRows[0].ReferenceAmount)

	next, err := GetProviderChannelBillingConfig(ctx, channelID, end+1)
	require.NoError(t, err)
	require.NotNil(t, next.Discount)
	assert.Equal(t, "0.7", next.Discount.Value.String())
	assert.Empty(t, next.Discount.Changes)

	require.NoError(t, save("0.85", 3, boundary))
	page, err = GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{Level: "url_channels", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, page.URLChannels, 1)
	require.NotNil(t, page.URLChannels[0].Discount)
	require.Len(t, page.URLChannels[0].Discount.Changes, 1)
	assert.Equal(t, "0.85", page.URLChannels[0].Discount.Changes[0].Value.String())
	require.NotNil(t, page.Totals.ReferenceAmount)
	assert.EqualValues(t, 175, *page.Totals.ReferenceAmount)

	require.NoError(t, save("0.75", 4, 0))
	page, err = GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{Level: "url_channels", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, page.URLChannels, 1)
	require.NotNil(t, page.URLChannels[0].Discount)
	assert.Empty(t, page.URLChannels[0].Discount.Changes)
	require.NotNil(t, page.Totals.ReferenceAmount)
	assert.EqualValues(t, 150, *page.Totals.ReferenceAmount)
}

func TestPublishedBillingReportKeepsURLScopeAndAppliesCurrentCoefficientPerRow(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	urlA, urlB := "https://first.example/v1", "https://second.example/v1"
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "account", BaseURL: &urlA}).Error)
	require.NoError(t, db.Create(&Channel{Id: 22, Name: "second account", BaseURL: &urlA}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, ModelName: "other", ChannelId: 22, Type: LogTypeConsume, CreatedAt: start + 12, Quota: 5, RequestId: "other-channel", Other: `{"group_ratio":1,"contract_applicable":false,"model_ratio":1}`}).Error)
	require.NoError(t, db.Create(&ProviderChannelBillingDiscount{ChannelId: 21, PeriodStart: start, Version: 1, Discount: decimal.RequireFromString("0.75")}).Error)
	metadata := []string{
		`{"group_ratio":1,"contract_applicable":false,"model_ratio":1,"completed_image_count":2}`,
		`{"group_ratio":1,"contract_applicable":false,"model_ratio":1,"image_count":9}`,
		`{"group_ratio":1,"contract_applicable":false,"model_ratio":1}`,
	}
	for i, at := range []int64{start + 10, start + 11, start + billingReportDaySeconds + 10} {
		require.NoError(t, db.Create(&Log{UserId: 7, ModelName: "chat", ChannelId: 21, Type: LogTypeConsume, CreatedAt: at, Quota: 1, RequestId: []string{"a", "b", "c"}[i], Other: metadata[i]}).Error)
	}
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", 21).Update("base_url", urlB).Error)
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Where("channel_id = ?", 21).Updates(map[string]any{"discount": decimal.RequireFromString("0.4"), "version": 2}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start+billingReportDaySeconds, "billing-test", "runner", nil, nil))
	pinned, _, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	result, err := getProviderBillingURLSummary(pinned, start, end, start, urlA, 21)
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	group := result.Groups[0]
	assert.Equal(t, urlA, group.UrlKey)
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 0, *group.ReferenceAmount, "two separate 1-quota rows each round to 0 at the frozen 0.4 coefficient")
	page, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{
		Level: "channels", GroupByURL: true, SortBy: "requests", Page: 1, PageSize: 1,
	})
	require.NoError(t, err)
	require.Len(t, page.URLGroups, 1)
	require.NotNil(t, page.Totals)
	assert.Equal(t, urlA, page.URLGroups[0].UrlKey)
	assert.Equal(t, 2, page.Total)
	assert.EqualValues(t, 4, page.Totals.Usage.Requests)
	assert.EqualValues(t, 2, page.Totals.Usage.CompletedImages, "a legacy billing count can be a reservation")
	require.NotNil(t, page.Totals.OriginalAmount)
	assert.EqualValues(t, 8, *page.Totals.OriginalAmount)

	filtered, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{
		Level: "channels", GroupByURL: true, Search: "second.example", Page: 1, PageSize: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, filtered.Totals)
	assert.EqualValues(t, 1, filtered.Totals.Usage.Requests)
	assert.Zero(t, filtered.Totals.Usage.CompletedImages)

	filter := UpstreamSummaryPageFilter{Level: "url_channels", SortBy: "requests", Page: 1, PageSize: 1}
	first, err := GetUpstreamSummaryPage(pinned, start, end, start, filter)
	require.NoError(t, err)
	require.Len(t, first.URLChannels, 1)
	assert.Equal(t, 3, first.Total)
	assert.Equal(t, urlA, first.URLChannels[0].URLKey)
	assert.Equal(t, 21, first.URLChannels[0].ChannelId)
	assert.EqualValues(t, 2, first.URLChannels[0].Usage.Requests)
	require.NotNil(t, first.Totals)
	assert.EqualValues(t, 4, first.Totals.Usage.Requests)
	require.NotNil(t, first.Totals.OriginalAmount)
	assert.EqualValues(t, 8, *first.Totals.OriginalAmount)

	filter.Page = 2
	second, err := GetUpstreamSummaryPage(pinned, start, end, start, filter)
	require.NoError(t, err)
	require.Len(t, second.URLChannels, 1)
	assert.Equal(t, urlA, second.URLChannels[0].URLKey)
	assert.Equal(t, 22, second.URLChannels[0].ChannelId)
	assert.Equal(t, first.Totals, second.Totals)

	filter.Page = 1
	filter.Search = "second account"
	matched, err := GetUpstreamSummaryPage(pinned, start, end, start, filter)
	require.NoError(t, err)
	require.Len(t, matched.URLChannels, 1)
	assert.Equal(t, 22, matched.URLChannels[0].ChannelId)
	require.NotNil(t, matched.Totals)
	assert.EqualValues(t, 1, matched.Totals.Usage.Requests)

	models, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{
		Level: "url_models", ReportURL: urlA, ChannelID: 21, Search: "chat", Page: 1, PageSize: 1,
	})
	require.NoError(t, err)
	require.Len(t, models.ModelRows, 1)
	assert.Equal(t, 1, models.Total)
	assert.Equal(t, 21, models.ModelRows[0].ChannelID)
	assert.EqualValues(t, 2, models.ModelRows[0].Usage.Requests)
	assert.EqualValues(t, 2, models.ModelRows[0].Usage.CompletedImages)
	require.NotNil(t, models.ModelRows[0].Discount)
	assert.Equal(t, "0.4", models.ModelRows[0].Discount.Value.String())

	updated := ProviderChannelBillingDiscount{ChannelId: 21, PeriodStart: start, Discount: decimal.RequireFromString("0.6")}
	require.NoError(t, SaveProviderChannelBillingDiscount(&updated, 2, 7))
	_, latest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	assert.Equal(t, BillingReportFromContext(pinned).Version, latest.Version)

	filter.Search = ""
	updatedPage, err := GetUpstreamSummaryPage(pinned, start, end, start, filter)
	require.NoError(t, err)
	require.Len(t, updatedPage.URLChannels, 1)
	updatedChannel := updatedPage.URLChannels[0]
	require.NotNil(t, updatedChannel.Discount)
	assert.EqualValues(t, 3, updatedChannel.Discount.Version)
	assert.Equal(t, "0.6", updatedChannel.Discount.Value.String())
	require.NotNil(t, updatedChannel.ReferenceAmount)
	assert.EqualValues(t, 2, *updatedChannel.ReferenceAmount, "round each 1-quota row before summing")
	require.NotNil(t, updatedPage.Totals.ReferenceAmount)
	assert.EqualValues(t, 8, *updatedPage.Totals.ReferenceAmount, "total includes both URLs and the other channel beyond page one")
	assert.Equal(t, first.Totals.OriginalAmount, updatedPage.Totals.OriginalAmount)

	updatedModels, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{
		Level: "url_models", ReportURL: urlA, ChannelID: 21, Page: 1, PageSize: 1,
	})
	require.NoError(t, err)
	require.Len(t, updatedModels.ModelRows, 1)
	assert.Equal(t, updatedChannel.Discount, updatedModels.ModelRows[0].Discount)
	assert.Equal(t, updatedChannel.ReferenceAmount, updatedModels.ModelRows[0].ReferenceAmount)

	missing, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{
		Level: "url_models", ReportURL: urlB, ChannelID: 22, Page: 1, PageSize: 1,
	})
	require.NoError(t, err)
	assert.Empty(t, missing.ModelRows)
	assert.Zero(t, missing.Total)

	detail, err := GetUpstreamBillingDetails(pinned, UpstreamBillingDetailFilter{Start: start, End: end, ChannelIds: []int{21}, URLKey: urlA}, 1, 20, false)
	require.NoError(t, err)
	require.Len(t, detail.Items, 2)
	assert.Equal(t, "a", detail.Items[0].RequestId)
	assert.Equal(t, "b", detail.Items[1].RequestId)
}

func TestPublishedBillingReportQueuesLateOccurrenceDate(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	today := start + 40*billingReportDaySeconds
	require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: start + 5, Quota: 100, RequestId: "before"}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, DiscoverLateBillingReportDays(ctx, today))
	require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeRefund, CreatedAt: start + 6, Quota: 30, RequestId: "late"}).Error)
	require.NoError(t, DiscoverLateBillingReportDays(ctx, today))
	var pending billingReportRebuild
	require.NoError(t, db.Where("start = ?", start).Take(&pending).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, CompleteBillingReportRebuild(ctx, start, pending.RequestedAt))
	require.NoError(t, DiscoverLateBillingReportDays(ctx, today))
	var remaining int64
	require.NoError(t, db.Model(&billingReportRebuild{}).Count(&remaining).Error)
	assert.Zero(t, remaining)
}

func TestPublishedLegacyBillingReportUsesBookedAmountsForUndiscountedRows(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "account"}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 187, ChannelId: 21, ModelName: "video", Type: LogTypeConsume, CreatedAt: start + 20, Quota: 932249, Other: `{"group_ratio":1,"contract_applicable":false,"is_task":true,"task_billing_event":"create"}`},
		{UserId: 7, TokenId: 187, ChannelId: 21, ModelName: "video", Type: LogTypeRefund, CreatedAt: start + 30, Quota: 863191, Other: `{"group_ratio":1,"contract_applicable":false,"is_task":true,"task_billing_event":"adjustment"}`},
		{UserId: 7, TokenId: 187, ChannelId: 21, ModelName: "image", Type: LogTypeConsume, CreatedAt: start + 2*billingReportDaySeconds + 20, Quota: 9038, Other: `{"group_ratio":1,"contract_applicable":false,"model_ratio":1}`},
		{UserId: 7, TokenId: 188, ChannelId: 21, ModelName: "image", Type: LogTypeConsume, CreatedAt: start + 20, Quota: 80, Other: `{"group_ratio":0.8,"contract_applicable":false,"model_ratio":1}`},
	}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildBillingReportDay(ctx, start+2*billingReportDaySeconds, "billing-test", "runner", nil, nil))

	var days []billingReportCustomerDay
	require.NoError(t, db.Where("user_id = ?", 7).Find(&days).Error)
	require.Len(t, days, 2)
	for _, day := range days {
		var details billingReportCustomerDetails
		require.NoError(t, common.UnmarshalJsonStr(string(day.Leaves), &details))
		for _, view := range []*billingReportCustomerDimension{&details.Key, &details.Channel} {
			view.RecordedAmounts = false
			for i := range view.Leaves {
				price := &view.Leaves[i].Price
				price.RecordedAmounts, price.Known, price.Complete = false, true, true
				price.Original = decimal.NewFromInt(999999)
			}
			for i := range view.Combinations {
				row := &view.Combinations[i]
				row.Complete, row.Original = true, decimal.NewFromInt(999999)
				stale := int64(999999)
				row.Combination.OriginalQuota, row.Combination.DiscountQuota = &stale, &stale
			}
		}
		raw, err := common.Marshal(details)
		require.NoError(t, err)
		var overview billingReportCustomerOverview
		require.NoError(t, common.UnmarshalJsonStr(string(day.Summary), &overview))
		overview.Price.RecordedAmounts = false
		overview.Price.Original = decimal.NewFromInt(999999)
		summary, err := common.Marshal(overview)
		require.NoError(t, err)
		require.NoError(t, db.Model(&day).Updates(map[string]any{
			"leaves": BillingStatementJSON(raw), "summary": BillingStatementJSON(summary),
		}).Error)
	}
	var lines []billingReportLine
	require.NoError(t, db.Where("user_id = ?", 7).Find(&lines).Error)
	for _, line := range lines {
		var record billingReportCustomerLine
		require.NoError(t, common.UnmarshalJsonStr(string(line.Customer), &record))
		record.Export.RecordedOriginal = ""
		record.Export.OriginalEstimate = "999999"
		record.Export.QualityStatus = "estimate"
		raw, err := common.Marshal(record)
		require.NoError(t, err)
		require.NoError(t, db.Model(&line).Update("customer", BillingStatementJSON(raw)).Error)
	}

	pinned, _, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	// The pinned result must only use the published evidence.
	require.NoError(t, db.Model(&Log{}).Where("user_id = ?", 7).Update("other", "{}").Error)
	statement, err := GetBillingCustomerStatement(pinned, 7, start, end, "api_key", 187, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 78096, statement.Summary.NetQuota)
	require.NotNil(t, statement.OriginalQuota)
	require.NotNil(t, statement.DiscountQuota)
	assert.EqualValues(t, 78096, *statement.OriginalQuota)
	assert.Zero(t, *statement.DiscountQuota)
	require.Len(t, statement.Groups, 1)
	for _, model := range statement.Groups[0].Models {
		require.NotNil(t, model.OriginalQuota)
		assert.Equal(t, model.Usage.NetQuota, *model.OriginalQuota)
	}
	require.Len(t, statement.DiscountCombinations, 2)
	for _, combination := range statement.DiscountCombinations {
		require.NotNil(t, combination.OriginalQuota)
		assert.Equal(t, combination.Usage.NetQuota, *combination.OriginalQuota)
	}
	all, err := GetBillingCustomerStatement(pinned, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.Nil(t, all.OriginalQuota)
	assert.EqualValues(t, 78176, all.Summary.NetQuota)
	for _, combination := range all.DiscountCombinations {
		if combination.GroupId == 188 {
			assert.Nil(t, combination.OriginalQuota)
			assert.Nil(t, combination.DiscountQuota)
		}
	}

	imageDay := start + 2*billingReportDaySeconds
	dayCtx, _, err := ResolveBillingReport(ctx, imageDay, imageDay+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	list, err := GetBillingCustomerStatementList(dayCtx, imageDay, imageDay+billingReportDaySeconds-1, "", "", "", "", 1, 10)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.NotNil(t, list.Items[0].OriginalQuota)
	assert.EqualValues(t, 9038, *list.Items[0].OriginalQuota)

	tokenID := 187
	details, err := GetBillingStatementLogs(pinned, BillingStatementLogFilter{
		UserId: 7, Start: start, End: end, TokenId: &tokenID,
	}, 1, 10, common.RoleAdminUser)
	require.NoError(t, err)
	require.Len(t, details.Items, 3)
	assert.EqualValues(t, 78096, details.Quota)
	for i, expected := range []string{"9038", "-863191", "932249"} {
		var detail struct {
			Facts CustomerExportRow `json:"billing_facts"`
		}
		require.NoError(t, common.UnmarshalJsonStr(details.Items[i].Other, &detail))
		assert.Equal(t, expected, detail.Facts.RecordedOriginal)
		assert.True(t, detail.Facts.OriginalExact)
	}

	exports, _, err := ReadBillingReportCustomerExportBatch(pinned, 7, CustomerExportFilters{StartTimestamp: start, EndTimestamp: end + 1}, 0, 100)
	require.NoError(t, err)
	require.Len(t, exports, 4)
	assert.Equal(t, "932249", exports[0].RecordedOriginal)
	assert.Equal(t, "-863191", exports[1].RecordedOriginal)
	assert.Equal(t, "9038", exports[2].RecordedOriginal)
	assert.Empty(t, exports[3].RecordedOriginal)
}

func TestPinnedBillingReportRetainsCoverageIssues(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, db.Create(&dailyReportRetry{Start: start, Kind: DailyReportBilling, CoverageIssue: true}).Error)
	_, latest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	require.Equal(t, 1, latest.IncompleteDays)
	_, pinned, err := ResolveBillingReport(ctx, start, end, latest.Version)
	require.NoError(t, err)
	assert.Equal(t, latest.IncompleteDays, pinned.IncompleteDays)
	assert.Equal(t, latest.PublishedDays, pinned.PublishedDays)
}

func TestPublishedBillingReportPrefersRecordedUpstreamAmounts(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	const channelID = 51
	url := "https://billing.example/v1"
	require.NoError(t, db.Create(&Channel{Id: channelID, Name: "provider", BaseURL: &url}).Error)
	require.NoError(t, db.Create(&ProviderChannelBillingDiscount{ChannelId: channelID, PeriodStart: start, Version: 1, Discount: decimal.RequireFromString("0.5")}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ChannelId: channelID, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "model", Quota: 100, Other: `{"group_ratio":0.8,"contract_applicable":false,"model_price":1,"admin_info":{"statement_original_quota":127}}`},
		{UserId: 7, ChannelId: channelID, CreatedAt: start + 2, Type: LogTypeRefund, ModelName: "model", Quota: 30, Other: `{"group_ratio":0.8,"contract_applicable":false,"model_price":1,"admin_info":{"statement_original_quota":39}}`},
		{UserId: 7, ChannelId: channelID, CreatedAt: start + 3, Type: LogTypeConsume, ModelName: "model", Quota: 0, Other: `{"model_ratio":1,"admin_info":{"statement_original_quota":5}}`},
		{UserId: 7, ChannelId: channelID, CreatedAt: start + 4, Type: LogTypeConsume, ModelName: "model", Quota: 4, Other: `{"group_ratio":0.8,"contract_applicable":false,"model_ratio":1}`},
	}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	// Exercise the same page through the chunked report storage path.
	var projection billingReportUpstreamDay
	require.NoError(t, db.Where("channel_id = ?", channelID).Take(&projection).Error)
	raw := string(projection.Payload)
	require.Greater(t, len(raw), 32)
	var parts []dailyReportPayloadPart
	for i, offset := 0, 0; offset < len(raw); i, offset = i+1, offset+16 {
		parts = append(parts, dailyReportPayloadPart{Generation: projection.Generation, Kind: reportBillingUpstreamPayload,
			ScopeID: channelID, Part: i, Payload: BillingStatementJSON(raw[offset:min(offset+16, len(raw))])})
	}
	require.NoError(t, db.CreateInBatches(parts, 100).Error)
	require.NoError(t, db.Model(&projection).Update("payload", "").Error)

	pinned, _, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	result, err := GetUpstreamSummaryPage(pinned, start, end, start, UpstreamSummaryPageFilter{Level: "url_channels", Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, result.URLChannels, 1)
	row := result.URLChannels[0]
	assert.EqualValues(t, 1, row.ModelCount, "two billing modes of one upstream model count once")
	assert.EqualValues(t, 1, result.ModelCount)
	require.NotNil(t, row.OriginalAmount)
	assert.EqualValues(t, 98, *row.OriginalAmount)
	require.NotNil(t, row.ReferenceAmount)
	assert.EqualValues(t, 50, *row.ReferenceAmount, "64 - 20 + 3 + 3")
	assert.True(t, row.DataQuality.HasEstimatedOriginal)

	details, err := GetUpstreamBillingDetails(pinned, UpstreamBillingDetailFilter{Start: start, End: end, ChannelIds: []int{channelID}, URLKey: url}, 1, 20, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 4)
	for i, want := range []int64{127, -39, 5, 5} {
		require.NotNil(t, details.Items[i].OriginalAmount)
		assert.Equal(t, want, *details.Items[i].OriginalAmount)
		require.NotNil(t, details.Items[i].OriginalAmountEstimated)
		assert.Equal(t, i == 3, *details.Items[i].OriginalAmountEstimated)
	}
}

func TestPublishedCustomerBillingShowsMultipleRecordedDiscountsWithUnknownOriginal(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 8, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "text", Quota: 80, Other: `{"model_ratio":1,"group_ratio":0.8,"contract_applicable":false,"admin_info":{"statement_original_quota":100}}`},
		{UserId: 7, TokenId: 8, CreatedAt: start + 2, Type: LogTypeConsume, ModelName: "text", Quota: 100, Other: `{"model_ratio":1,"group_ratio":0.9,"contract_applicable":false,"tool_surcharges":[{}]}`},
	}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	pinned, _, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	statement, err := GetBillingCustomerStatement(pinned, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	row := statement.Groups[0].Models[0]
	assert.True(t, row.MultipleDiscounts)
	assert.Nil(t, row.DiscountRatio)
	assert.Nil(t, row.OriginalQuota)
	assert.EqualValues(t, 180, row.Usage.NetQuota)
}
