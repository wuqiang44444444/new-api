package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportCacheTiersPreserveObservedUsageThroughPricingAndMerge(t *testing.T) {
	anonymous := parseBillingReconciliationLog(billingReconciliationLog{Type: LogTypeConsume, Other: `{"cache_write_tokens":9}`})
	require.NotNil(t, anonymous.cacheWrite.tiers)
	assert.Nil(t, anonymous.cacheWrite.tiers.Standard)
	assert.EqualValues(t, 9, anonymous.cacheWrite.tiers.Unclassified)

	claude := parseBillingReconciliationLog(billingReconciliationLog{Type: LogTypeConsume, Other: `{"claude":true,"usage_semantic":"anthropic","cache_creation_tokens":20,"cache_creation_ratio":1.25}`})
	require.NotNil(t, claude.cacheWrite.tiers.Standard)
	assert.EqualValues(t, 20, *claude.cacheWrite.tiers.Standard)
	assert.Zero(t, claude.cacheWrite.tiers.Unclassified)

	split := parseBillingReconciliationLog(billingReconciliationLog{Type: LogTypeConsume, Other: `{"usage_semantic":"openai","cache_creation_tokens":5,"cache_creation_ratio":1.25,"cache_creation_tokens_12h":12}`})
	require.NotNil(t, split.cacheWrite.tiers)
	assert.Nil(t, split.cacheWrite.tiers.Standard)
	assert.EqualValues(t, 12, *split.cacheWrite.tiers.TwelveHours)

	text := parseBillingReconciliationLog(billingReconciliationLog{Type: LogTypeConsume, Other: `{"usage_semantic":"openai","cache_creation_ratio":1.25,"cache_creation_tokens":17,"cache_creation_tokens_5m":0,"cache_creation_tokens_12h":12}`})
	metrics := UsageAnalyticsMetrics{CacheWriteTokens: text.cacheWrite.total, CacheWrites: text.cacheWrite.tiers}
	setUpstreamExpressionTokenMeter(&metrics, &FrozenChargeReplay{TokenInputs: map[string]float64{"p": 100, "cc": 29, "cc12h": 0}}, 1)
	var total usageMetricsAcc
	total.mergeTokens(metrics)
	total.mergeTokens(UsageAnalyticsMetrics{CacheWriteTokens: anonymous.cacheWrite.total, CacheWrites: anonymous.cacheWrite.tiers})
	require.NotNil(t, total.m.CacheWrites)
	assert.EqualValues(t, 38, total.m.CacheWriteTokens)
	assert.EqualValues(t, 1, total.m.CacheWritePricingAdjustedRows)
	assert.EqualValues(t, 5, *total.m.CacheWrites.Standard)
	assert.Zero(t, *total.m.CacheWrites.FiveMinutes)
	assert.Nil(t, total.m.CacheWrites.OneHour)
	assert.EqualValues(t, 12, *total.m.CacheWrites.TwelveHours)
	assert.EqualValues(t, 9, total.m.CacheWrites.Unclassified)

	standalone := UsageAnalyticsMetrics{}
	setUpstreamExpressionTokenMeter(&standalone, &FrozenChargeReplay{TokenInputs: map[string]float64{"cc12h": 12, "p": 100}}, 1)
	assert.EqualValues(t, 112, standalone.InputTokens)
	assert.EqualValues(t, 12, standalone.CacheWriteTokens)
	assert.Empty(t, standalone.CacheWrites.Unclassified)
	assert.Nil(t, standalone.CacheWrites.TwelveHours)
}

func TestPublishedBillingRefundUsesEachReportsRequestID(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "channel"}).Error)
	original := Log{UserId: 7, TokenId: 4, ChannelId: 21, ModelName: "video", Type: LogTypeConsume,
		CreatedAt: start + 1, Quota: 87, RequestId: "charge-request", IsStream: true,
		Other: `{"model_ratio":1,"group_ratio":1,"contract_applicable":false}`}
	require.NoError(t, db.Create(&original).Error)
	metadata, err := common.Marshal(map[string]any{"admin_info": map[string]any{"original_preauth_log_id": original.Id}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ChannelId: 21, ModelName: "video", Type: LogTypeRefund,
		CreatedAt: start + 2, Quota: 87, RequestId: "refund-request", Other: string(metadata)}).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	pinned, _, err := ResolveBillingReport(ctx, start, start+86399, "")
	require.NoError(t, err)

	upstream, err := GetUpstreamBillingDetails(pinned,
		UpstreamBillingDetailFilter{Start: start, End: start + 86399, ChannelIds: []int{21}, RequestId: "refund-request"}, 1, 20, false)
	require.NoError(t, err)
	require.Len(t, upstream.Items, 1)
	assert.Equal(t, "refund-request", upstream.Items[0].RequestId)
	require.NotNil(t, upstream.Items[0].IsStream)
	assert.True(t, *upstream.Items[0].IsStream)

	page, err := ReadCustomerStatementExportEvents(pinned, db, 7,
		CustomerExportFilters{StartTimestamp: start, EndTimestamp: start + 86400, RequestId: "charge-request"}, 0, 20)
	require.NoError(t, err)
	require.Len(t, page.Events, 2)
	assert.EqualValues(t, -87, page.Events[1].Charge)
	assert.Equal(t, "charge-request", page.Events[1].Row.RequestId)
}

func TestHistoricalSummaryKeepsFrozenSecondsWithoutMeteringPrehold(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	seconds := decimal.RequireFromString("3.75")
	missing := false
	for i, row := range []CustomerExportRow{
		{CountsTowardBill: true, CustomerModel: "video", TokenName: "prod", BillingMode: BillingReconciliationModePerSecond,
			EventType: "consume", Quota: 20, BillableSecondsMissing: &missing},
		{CountsTowardBill: true, CustomerModel: "video", TokenName: "prod", BillingMode: BillingReconciliationModePerSecond,
			EventType: "refund", Quota: 5, BillableSeconds: &seconds},
	} {
		raw, err := common.Marshal(billingReportCustomerLine{Export: row})
		require.NoError(t, err)
		logType := LogTypeConsume
		if row.EventType == "refund" {
			logType = LogTypeRefund
		}
		require.NoError(t, db.Create(&billingReportLine{
			Generation: "historical", SourceID: int64(i + 1), UserID: 7, TokenID: 4, CreatedAt: start + int64(i), LogType: logType,
			Customer: BillingStatementJSON(raw),
		}).Error)
	}
	require.NoError(t, rebuildMissingHistoricalBillingCustomers(ctx, db, "historical"))
	var day billingReportCustomerDay
	require.NoError(t, db.Where("generation = ? AND user_id = ?", "historical", 7).Take(&day).Error)
	var summary billingReportCustomerOverview
	require.NoError(t, common.UnmarshalJsonStr(string(day.Summary), &summary))
	usage := summary.Price.Model.Usage
	require.NotNil(t, usage.BillableSeconds)
	assert.Equal(t, "3.75", usage.BillableSeconds.String())
	assert.Zero(t, usage.MissingBillableSeconds)
}
