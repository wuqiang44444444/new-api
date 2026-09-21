package model

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	usageTestDayStart   = int64(1789488000) // 2026-09-16 00:00 +08:00 (Wednesday)
	usageTestWeekStart  = int64(1789315200) // 2026-09-14 00:00 +08:00 (Monday)
	usageTestTokenQuota = 8800
)

// setupUsageAnalyticsTestDB extends the shared reconciliation fixture with the
// token table needed by stable API Key identity lookup.
func setupUsageAnalyticsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.AutoMigrate(&Token{}, &TaskBillingDelivery{}, &BatchJob{}, &BatchJobLine{}))
	return db
}

func TestUsageAnalyticsRetryAndEmptyResults(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	empty, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	assert.NotNil(t, empty.Keys)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, RequestId: "retry", CreatedAt: period.StartTimestamp + 10, Type: LogTypeError, ChannelId: 21, ModelName: "chat"},
		{UserId: 7, RequestId: "retry", CreatedAt: period.StartTimestamp + 20, Type: LogTypeConsume, ChannelId: 22, ModelName: "chat", Quota: 10},
		{UserId: 7, RequestId: "cross-day", CreatedAt: period.StartTimestamp - 1, Type: LogTypeError, ChannelId: 21, ModelName: "chat"},
		{UserId: 7, RequestId: "cross-day", CreatedAt: period.StartTimestamp + 30, Type: LogTypeConsume, ChannelId: 22, ModelName: "chat", Quota: 10},
	}).Error)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	assert.EqualValues(t, 2, view.Total.TotalCalls)
	assert.Zero(t, view.Total.FailureCalls)
	previous, err := ResolveUsageAnalyticsPeriod("day", "2026-09-15", 0)
	require.NoError(t, err)
	view, err = GetUsageCustomerView(context.Background(), previous, 7)
	require.NoError(t, err)
	assert.Zero(t, view.Total.TotalCalls)
}

func TestUsageAnalyticsSettledTaskWithUndeliveredRefund(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	task := Task{ID: 100, TaskID: "pending-log", UserId: 7, ChannelId: 21, Platform: "video", Status: TaskStatusSuccess,
		FinishTime: period.StartTimestamp + 10, BillingState: TaskBillingStateSettled, Quota: 80,
		Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 11,
			AsyncBilling: &TaskAsyncBillingContext{ActualUsageReported: true, ActualTokens: 0}}}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, RequestId: "task-billing:100:create", Type: LogTypeConsume, Quota: 100}).Error)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	assert.EqualValues(t, 80, view.Total.NetQuota)
	assert.EqualValues(t, 1, view.Total.RowsMissingMoney)
	assert.Zero(t, view.Total.RowsMissingTokens, "explicit reported zero is known")
	require.Len(t, view.Keys, 1)
	assert.Equal(t, 11, view.Keys[0].TokenId)
}

func TestUsageAnalyticsDiscountReadFailureIsNotDefaultOne(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&ProviderChannelBillingDiscount{}))
	_, err = GetUsageUpstreamView(context.Background(), period)
	require.Error(t, err)
}

func TestUsageCustomerViewCountsSyncCallsRefundsAndMissingEvidence(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer", Quota: usageTestTokenQuota}).Error)
	require.NoError(t, db.Create(&Token{Id: 11, Name: "key-a", UserId: 7}).Error)
	logs := []Log{
		// Successful token call: input 100 with 30 cache tokens under the
		// recorded OpenAI semantics — input stays 100, never 130.
		{UserId: 7, CreatedAt: usageTestDayStart + 100, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "gpt", PromptTokens: 100, CompletionTokens: 40, Quota: 1200, Other: `{"contract_applicable":false,"usage_semantic":"openai","group_ratio":1,"model_ratio":1,"cache_tokens":30}`},
		// Failure rows write zero base fields without usage evidence: counted,
		// reported as missing usage, never as an actual zero.
		{UserId: 7, CreatedAt: usageTestDayStart + 200, Type: LogTypeError, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "gpt"},
		// Refund without task linkage is a money fact only — no call count.
		{UserId: 7, CreatedAt: usageTestDayStart + 300, Type: LogTypeRefund, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "gpt", Quota: 300, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		// Another user's rows must not leak into the scoped view.
		{UserId: 8, CreatedAt: usageTestDayStart + 400, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "gpt", PromptTokens: 999, Quota: 999, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", usageTestDayStart+3600)
	require.NoError(t, err)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)

	total := view.Total
	assert.EqualValues(t, 2, total.TotalCalls)
	assert.EqualValues(t, 1, total.SuccessCalls)
	assert.EqualValues(t, 1, total.FailureCalls)
	assert.EqualValues(t, 0, total.CancelledCalls)
	assert.EqualValues(t, 100, total.InputTokens)
	assert.EqualValues(t, 30, total.CacheReadTokens)
	assert.EqualValues(t, 40, total.OutputTokens)
	assert.EqualValues(t, 1200, total.GrossQuota)
	assert.EqualValues(t, 1200, total.NetQuota)
	// The failure row is the missing-usage row; the unlinked refund is listed
	// separately instead of pretending it belongs to a call.
	assert.EqualValues(t, 1, total.RowsMissingTokens)
	assert.EqualValues(t, 300, total.UnlinkedRefundQuota)
	require.Len(t, view.Keys, 1)
	assert.Equal(t, "key-a", view.Keys[0].TokenName)
	require.Len(t, view.Keys[0].Models, 1)
	assert.Equal(t, "gpt", view.Keys[0].Models[0].ModelName)
	assert.EqualValues(t, 2, view.Keys[0].Models[0].Total.TotalCalls)
	// Day totals and leaf totals must stay consistent with the range total.
	assert.EqualValues(t, total.TotalCalls, view.DayTotals[0].TotalCalls)
	assert.EqualValues(t, total.NetQuota, view.DayTotals[0].NetQuota)
}

func TestUsageCustomerViewAttributesTaskToFinishDay(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer", Quota: usageTestTokenQuota}).Error)
	const taskRowID = int64(100)
	// Task finishes Tuesday 23:59:50; its create preconsume was written on
	// Monday and the settle adjustment lands in the next week.
	task := Task{
		ID: taskRowID, TaskID: "task_public_1", Platform: "video", UserId: 7, ChannelId: 21,
		Status: TaskStatusSuccess, SubmitTime: usageTestWeekStart + 100,
		FinishTime: usageTestWeekStart + 86400 + 86390, BillingState: TaskBillingStateSettled, Quota: 150,
		Properties: Properties{OriginModelName: "video-model"},
	}
	require.NoError(t, db.Create(&task).Error)
	// Delivery logs are keyed by the deterministic request id and may land on
	// any day — they always follow the task finish day.
	deliveries := []Log{
		{UserId: 7, CreatedAt: usageTestWeekStart + 100, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "video-model", Quota: 100, RequestId: "task-billing:100:create", Other: `{"is_task":true,"task_id":"task_public_1","contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: usageTestWeekStart + 8*86400, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "video-model", Quota: 50, CompletionTokens: 700, RequestId: "task-billing:100:adjustment", Other: `{"task_id":"task_public_1","pre_consumed_quota":100,"actual_quota":150,"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&deliveries).Error)
	// A sync call on the finish day proves the task is not counted twice and
	// that both contribute to the same day bucket.
	sync := Log{UserId: 7, CreatedAt: usageTestWeekStart + 86400 + 800, Type: LogTypeConsume, TokenId: 12, TokenName: "key-b", ChannelId: 22, ModelName: "chat", PromptTokens: 10, Quota: 20, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`}
	require.NoError(t, db.Create(&sync).Error)

	period, err := ResolveUsageAnalyticsPeriod("week", "2026-09-16", usageTestWeekStart+8*86400)
	require.NoError(t, err)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)

	total := view.Total
	assert.EqualValues(t, 2, total.TotalCalls)
	assert.EqualValues(t, 2, total.SuccessCalls)
	assert.EqualValues(t, 150, total.GrossQuota-20)
	assert.EqualValues(t, 700, total.OutputTokens)
	// The video-model leaf exists exactly once, with its finish-day bucket filled.
	require.Len(t, view.Keys, 2)
	var videoRow *UsageCustomerModelRow
	for i := range view.Keys {
		for j := range view.Keys[i].Models {
			if view.Keys[i].Models[j].ModelName == "video-model" {
				videoRow = &view.Keys[i].Models[j]
			}
		}
	}
	require.NotNil(t, videoRow)
	assert.EqualValues(t, 1, videoRow.Total.TotalCalls)
	assert.EqualValues(t, 150, videoRow.Total.GrossQuota)
	assert.EqualValues(t, 700, videoRow.Total.OutputTokens)
	assert.EqualValues(t, 1, videoRow.Days[1].TotalCalls)
	for i, day := range videoRow.Days {
		if i == 1 {
			continue
		}
		assert.EqualValues(t, 0, day.TotalCalls, "no task call may appear on day %d", i)
	}
}

func TestUsageUpstreamViewGroupsByUrlAndMonthlyDiscounts(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 71, Name: "alpha", BaseURL: urlPtr("https://core.example.com")},
		{Id: 72, Name: "beta", BaseURL: urlPtr("https://core.example.com/")},
		{Id: 73, Name: "gamma", BaseURL: urlPtr("https://core.example.com")},
	}).Error)
	// Natural week 2026-10-26..2026-11-01 crosses the month boundary; the
	// coefficient must be applied per record month, never averaged.
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-11-01", 0)
	require.NoError(t, err)
	oct1 := usageMonthStart(period.Days[0].Start)
	nov1 := usageMonthStart(period.Days[6].Start)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: oct1, ChannelId: 71, Discount: decimal.RequireFromString("0.8"), Reason: "contract a"}, 0, 9))
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: nov1, ChannelId: 71, Discount: decimal.RequireFromString("0.9"), Reason: "contract b"}, 0, 9))
	logs := []Log{
		// October, channel 71: original 1000 -> reference 800.
		{UserId: 7, CreatedAt: period.Days[1].Start + 100, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 71, ModelName: "shared", PromptTokens: 10, Quota: 1000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		// November, channel 71: original 500 -> reference 450.
		{UserId: 7, CreatedAt: period.Days[6].Start + 100, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 71, ModelName: "shared", PromptTokens: 5, Quota: 500, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		// October, channel 72 (no config -> default coefficient 1): 2000 -> 2000.
		{UserId: 7, CreatedAt: period.Days[1].Start + 250, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 72, ModelName: "shared", PromptTokens: 20, Quota: 2000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		// Native channel test on channel 73: usage stays, money stays out.
		{UserId: 7, CreatedAt: period.Days[1].Start + 300, Type: LogTypeConsume, TokenId: 0, TokenName: "模型测试", ChannelId: 73, ModelName: "shared", PromptTokens: 7, Quota: 999, Content: "模型测试", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	upstream, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	require.Len(t, upstream.UrlGroups, 1)
	group := upstream.UrlGroups[0]
	assert.Equal(t, "https://core.example.com", group.BaseURL)
	require.Len(t, group.Models, 1)
	require.Len(t, group.Models[0].Channels, 3)
	byChannel := make(map[int]UsageUpstreamChannelRow)
	for _, channel := range group.Models[0].Channels {
		byChannel[channel.ChannelId] = channel
	}
	leaf71 := byChannel[71].Total
	assert.EqualValues(t, 2, leaf71.TotalCalls)
	assert.NotNil(t, leaf71.OriginalQuotaEstimate)
	assert.EqualValues(t, 1500, *leaf71.OriginalQuotaEstimate)
	assert.NotNil(t, leaf71.ReferenceAmount)
	assert.EqualValues(t, 1250, *leaf71.ReferenceAmount)
	// The channel row lists both month coefficients with their sources.
	require.Len(t, byChannel[71].Discounts, 2)
	assert.True(t, byChannel[71].Total.MultipleDiscounts)
	assert.EqualValues(t, 1, byChannel[72].Total.TotalCalls)
	require.NotNil(t, byChannel[72].Total.ReferenceAmount)
	assert.EqualValues(t, 2000, *byChannel[72].Total.ReferenceAmount)
	// Channel tests are usage-only: counted in calls, absent from money.
	assert.EqualValues(t, 1, byChannel[73].Total.TotalCalls)
	assert.EqualValues(t, 1, byChannel[73].Total.UsageOnlyRows)
	assert.Nil(t, byChannel[73].Total.OriginalQuotaEstimate)
	assert.Nil(t, byChannel[73].Total.ReferenceAmount)
	// Batch rows keep their trusted row-level counts; the job never adds an
	// extra call on top of its line count.
	batchLog := Log{UserId: 7, CreatedAt: period.Days[2].Start + 100, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 72, ModelName: "batch-model", Quota: 3000, Other: `{"billing_mode":"azure_batch","task_id":"task_batch_1","task_billing_event":"create","batch_line_count":3,"input_tokens_total":300,"output_tokens_total":200,"usage_semantic":"openai","contract_applicable":false,"group_ratio":1,"model_ratio":1}`}
	require.NoError(t, db.Create(&batchLog).Error)
	batchTask := Task{ID: 900, TaskID: "task_batch_1", Platform: "azure_batch", UserId: 7, ChannelId: 72, Status: TaskStatusSuccess, FinishTime: period.Days[2].Start + 200, Quota: 3000}
	require.NoError(t, db.Create(&batchTask).Error)
	require.NoError(t, db.Create(&BatchJob{Id: "batch-900", TaskRowId: 900, UserId: 7, ChannelId: 72, TokenId: 11, PublicModel: "batch-model", UpstreamStatus: "completed", CompletedAt: period.Days[2].Start + 200, CountTotal: 3, CountCompleted: 2, CountFailed: 1, UsageInput: 300, UsageOutput: 200, DeliveryState: BatchDeliveryReady}).Error)
	refreshed, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	for _, g := range refreshed.UrlGroups {
		for _, m := range g.Models {
			if m.ProviderModel == "batch-model" {
				require.Len(t, m.Channels, 1)
				assert.EqualValues(t, 3, m.Total.TotalCalls)
				assert.EqualValues(t, 2, m.Total.SuccessCalls)
				assert.EqualValues(t, 1, m.Total.FailureCalls)
				assert.EqualValues(t, 3, m.Total.RowsMoneyPending)
				assert.EqualValues(t, 300, m.Total.InputTokens)
				assert.EqualValues(t, 200, m.Total.OutputTokens)
			}
		}
	}

}

func TestUsageAnalyticsTaskFinishMonthAndFrozenDiscount(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-11-01", 0)
	require.NoError(t, err)
	require.NoError(t, db.Create(&Channel{Id: 71, Name: "upstream", BaseURL: urlPtr("https://example.com")}).Error)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: usageMonthStart(period.StartTimestamp), ChannelId: 71, Discount: decimal.RequireFromString("0.8")}, 0, 9))
	snapshot, err := FreezeUsageAnalyticsDiscounts(context.Background(), period)
	require.NoError(t, err)
	task := Task{ID: 201, TaskID: "cross-month", UserId: 7, ChannelId: 71, Platform: "video", Status: TaskStatusSuccess, FinishTime: period.StartTimestamp + 20, BillingState: TaskBillingStateSettled, Quota: 100, Properties: Properties{OriginModelName: "video"}}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, RequestId: "task-billing:201:create", CreatedAt: period.StartTimestamp - 86400, ChannelId: 71, Type: LogTypeConsume, Quota: 100, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`}).Error)
	view, err := GetUsageUpstreamView(context.Background(), period, snapshot)
	require.NoError(t, err)
	require.NotNil(t, view.Total.ReferenceAmount)
	assert.EqualValues(t, 80, *view.Total.ReferenceAmount)
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Where("channel_id = ?", 71).Update("discount", decimal.RequireFromString("0.5")).Error)
	frozen, err := GetUsageUpstreamView(context.Background(), period, snapshot)
	require.NoError(t, err)
	require.NotNil(t, frozen.Total.ReferenceAmount)
	assert.EqualValues(t, 80, *frozen.Total.ReferenceAmount)
	current, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	require.NotNil(t, current.Total.ReferenceAmount)
	assert.EqualValues(t, 50, *current.Total.ReferenceAmount)
}

func TestUsageAnalyticsCountsOnlyRecordedUpstreamFailures(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, RequestId: "failed", UpstreamRequestId: "provider-attempt", ChannelId: 21, CreatedAt: period.StartTimestamp + 1, Type: LogTypeError, ModelName: "chat"},
		{UserId: 7, RequestId: "local", ChannelId: 21, CreatedAt: period.StartTimestamp + 2, Type: LogTypeError, ModelName: "chat"},
	}).Error)
	view, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	assert.EqualValues(t, 1, view.Total.FailureCalls)
	assert.EqualValues(t, 1, view.Total.RowsMissingTokens)
	assert.Nil(t, view.Total.OriginalQuotaEstimate)
}

func TestUsageAnalyticsOverviewSearchBeforeLimit(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "match-me"}).Error)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	agg := newUsageAggregation(period, usageViewOptions{WantOverview: true})
	for id := 1; id <= maxUsageAnalyticsCustomers+1; id++ {
		acc := &usageMetricsAcc{}
		acc.addCalls(usageResultSuccess, 1)
		agg.mergeOverview(usageOverviewKey{user: id, day: 0}, acc)
	}
	overview := UsageCustomersOverview{}
	require.NoError(t, finalizeUsageCustomersOverview(context.Background(), &overview, agg, "match-me"))
	require.Len(t, overview.Customers, 1)
	assert.EqualValues(t, 1, overview.Total.TotalCalls)
	require.Error(t, finalizeUsageCustomersOverview(context.Background(), &UsageCustomersOverview{}, agg, ""))
}

func TestUsageAnalyticsTaskPartialPriceEvidenceStaysUnknown(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 1789920000)
	require.NoError(t, err)
	require.NoError(t, db.Create(&Task{ID: 301, TaskID: "partial-price", UserId: 7, ChannelId: 21, Platform: "video", Status: TaskStatusSuccess, FinishTime: period.StartTimestamp + 10, BillingState: TaskBillingStateSettled, Quota: 150}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ChannelId: 21, RequestId: "task-billing:301:create", Type: LogTypeConsume, Quota: 100, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, ChannelId: 21, RequestId: "task-billing:301:adjustment", Type: LogTypeConsume, Quota: 50},
	}).Error)
	view, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	assert.EqualValues(t, 150, view.Total.NetQuota)
	assert.Nil(t, view.Total.OriginalQuotaEstimate)
	assert.Nil(t, view.Total.ReferenceAmount)
	assert.NotEmpty(t, view.Total.EstimateReasons)
}

func TestUsageAnalyticsDeliveryUsesTaskRowIdentityAcrossApps(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 1789920000)
	require.NoError(t, err)
	tasks := []Task{
		{ID: 501, TaskID: "same-provider-id", UserId: 7, AppID: 1, Status: TaskStatusSuccess, FinishTime: period.StartTimestamp + 10, BillingState: TaskBillingStateSettled, Quota: 10, PrivateData: TaskPrivateData{TokenId: 11}},
		{ID: 502, TaskID: "same-provider-id", UserId: 7, AppID: 2, Status: TaskStatusSuccess, FinishTime: period.StartTimestamp + 20, BillingState: TaskBillingStateSettled, Quota: 20, PrivateData: TaskPrivateData{TokenId: 12}},
		{ID: 503, TaskID: "same-provider-id", UserId: 8, AppID: 1, Status: TaskStatusSuccess, FinishTime: period.StartTimestamp + 30, BillingState: TaskBillingStateSettled, Quota: 99},
	}
	require.NoError(t, db.Create(&tasks).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, RequestId: "task-billing:501:create", Type: LogTypeConsume, Quota: 10},
		{UserId: 7, RequestId: "task-billing:502:create", Type: LogTypeConsume, Quota: 20},
		{UserId: 8, RequestId: "task-billing:503:create", Type: LogTypeConsume, Quota: 99},
	}).Error)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	assert.EqualValues(t, 2, view.Total.TotalCalls)
	assert.EqualValues(t, 30, view.Total.NetQuota)
	require.Len(t, view.Keys, 2)
	for _, key := range view.Keys {
		assert.Zero(t, key.Total.RowsMissingMoney)
	}
}
