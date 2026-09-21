package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingDetailsEvidenceRowsAndFilters(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 81, Name: "evidence", BaseURL: urlPtr("https://detail.example.com")}).Error)
	require.NoError(t, db.Create(&Channel{Id: 82, Name: "other", BaseURL: urlPtr("https://other.example.com")}).Error)
	logs := []Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 81, ModelName: "chat", Quota: 400, RequestId: "req-1", UpstreamRequestId: "up-1", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 81, ModelName: "video", Quota: 100, RequestId: "req-2", UpstreamRequestId: "up-2", Other: `{"task_id":"t1","task_billing_event":"create","is_task":true,"contract_applicable":false,"group_ratio":1,"model_price":1}`},
		{UserId: 7, CreatedAt: 1102, Type: LogTypeRefund, ChannelId: 81, ModelName: "video", Quota: 30, RequestId: "req-2", UpstreamRequestId: "up-3", Other: `{"task_id":"t1","task_billing_event":"adjustment","actual_quota":30,"pre_consumed_quota":100,"contract_applicable":false,"group_ratio":1,"model_price":1}`},
		{UserId: 7, CreatedAt: 1103, Type: LogTypeRefund, ChannelId: 81, ModelName: "chat", Quota: 999, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		// 测试行保存原生计价版本与原价，计入参考金额并保留依据。
		{UserId: 8, CreatedAt: 1104, Type: LogTypeConsume, ChannelId: 81, ModelName: "chat", Quota: 1000, PromptTokens: 1000, TokenId: 0, TokenName: "模型测试", Content: "模型测试", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":1000}}`},
		{UserId: 9, CreatedAt: 1105, Type: LogTypeConsume, ChannelId: 82, ModelName: "chat", Quota: 55, RequestId: "req-9", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}}, 1, 50, true)
	require.NoError(t, err)
	// Refunds enter the signed money evidence; tests stay visible without money.
	require.Len(t, details.Items, 5)
	assert.EqualValues(t, 5, details.Total)
	require.Len(t, details.Items, 5)
	call := details.Items[0]
	assert.Equal(t, "req-1", call.RequestId)
	assert.Equal(t, "call", call.Event)
	assert.Equal(t, "up-1", call.UpstreamRequestId)
	require.NotNil(t, call.OriginalAmount)
	assert.EqualValues(t, 400, *call.OriginalAmount)
	create := details.Items[1]
	assert.Equal(t, "task_create", create.Event)
	assert.Equal(t, "t1", create.PlatformTaskId)
	adjustment := details.Items[2]
	assert.Equal(t, "task_adjustment", adjustment.Event)
	require.NotNil(t, adjustment.OriginalAmount)
	assert.EqualValues(t, -30, *adjustment.OriginalAmount)
	assert.Equal(t, "channel_test", details.Items[4].Event)
	require.NotNil(t, details.Items[4].OriginalAmount)
	assert.EqualValues(t, 1000, *details.Items[4].OriginalAmount)
	require.NotNil(t, details.Items[4].TestPricing)
	assert.Equal(t, "ratio", details.Items[4].TestPricing.Mode)
	assert.Equal(t, "priced", details.Items[4].TestPricing.Status)

	assert.Equal(t, "refund", details.Items[3].Event)
	require.NotNil(t, details.Items[3].OriginalAmount)
	assert.EqualValues(t, -999, *details.Items[3].OriginalAmount)
	second, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}}, 2, 2, false)
	require.NoError(t, err)
	require.Len(t, second.Items, 2)
	assert.Equal(t, details.Items[2].RowId, second.Items[0].RowId)
	assert.Equal(t, details.Items[3].RowId, second.Items[1].RowId)
	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	for _, group := range summary.Groups {
		if group.UrlKey == "https://detail.example.com" {
			require.NotNil(t, group.OriginalAmount)
			assert.EqualValues(t, 400+100-30-999+1000, *group.OriginalAmount)
		}
	}
	// Double-sided request id search.
	byUpstream, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}, UpstreamRequestId: "up-3"}, 1, 50, true)
	require.NoError(t, err)
	require.Len(t, byUpstream.Items, 1)
	assert.Equal(t, "task_adjustment", byUpstream.Items[0].Event)
	byLocal, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}, RequestId: "req-1"}, 1, 50, true)
	require.NoError(t, err)
	require.Len(t, byLocal.Items, 1)
	assert.Equal(t, "call", byLocal.Items[0].Event)

	// Provider model filter matches the upstream identity, not the customer name.
	byModel, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}, ProviderModel: "chat"}, 1, 50, false)
	require.NoError(t, err)
	require.Len(t, byModel.Items, 3)
	for _, item := range byModel.Items {
		assert.Empty(t, item.UpstreamTaskId, "non-root viewers never receive root-scoped ids")
	}

	// A URL grouping key resolves to its channels under the shared identity.
	channels, err := GetChannelIdsByNormalizedBaseURL("https://detail.example.com")
	require.NoError(t, err)
	assert.Equal(t, []int{81}, channels)
}

func TestUpstreamURLDetailsResolveTypeDefault(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	empty := ""
	channel := Channel{Id: 83, Name: "default endpoint", Type: 1, BaseURL: &empty}
	require.NoError(t, db.Create(&channel).Error)
	key, _ := billingURLGroupIdentity(channel, true, channel.Id)
	ids, err := GetChannelIdsByNormalizedBaseURL(key)
	require.NoError(t, err)
	assert.Equal(t, []int{83}, ids)
}

func TestUpstreamRefundUsesCustomerPreauthEvidence(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: 1100, Quota: 87, Other: `{"contract_applicable":false,"statement_snapshot":{"snapshot_version":1,"billing_mode":"per_second","customer_model":"video","group_ratio":0.87},"group_ratio":0.87}`}
	require.NoError(t, db.Create(&original).Error)
	refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1200, Quota: 87, Other: fmt.Sprintf(`{"admin_info":{"original_preauth_log_id":%d}}`, original.Id)}
	require.NoError(t, db.Create(&refund).Error)
	detail, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1150, End: 1300, ChannelIds: []int{3}}, 1, 50, false)
	require.NoError(t, err)
	require.Len(t, detail.Items, 1)
	assert.Equal(t, BillingReconciliationModePerSecond, detail.Items[0].BillingMode)
	require.NotNil(t, detail.Items[0].OriginalAmount)
	assert.EqualValues(t, -100, *detail.Items[0].OriginalAmount)
	summary, err := GetProviderBillingURLSummary(1000, 1300, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	require.NotNil(t, summary.Groups[0].OriginalAmount)
	assert.Zero(t, *summary.Groups[0].OriginalAmount)
}

func TestUpstreamDetailsKeepKnownAndFallbackIdentitiesSeparate(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ChannelId: 81, CreatedAt: 1100, Type: LogTypeConsume, ModelName: "same", Other: `{"model_ratio":1,"upstream_model_name":"same"}`},
		{UserId: 7, ChannelId: 81, CreatedAt: 1101, Type: LogTypeConsume, ModelName: "same", Other: `{"model_ratio":1,"is_model_mapped":true}`},
		{UserId: 7, ChannelId: 81, CreatedAt: 1102, Type: LogTypeConsume, ModelName: "different", Other: `{"model_ratio":1,"is_model_mapped":true}`},
	}).Error)
	for _, fallback := range []bool{false, true} {
		details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}, ProviderModel: "same", ProviderModelFallback: &fallback}, 1, 50, false)
		require.NoError(t, err)
		require.Len(t, details.Items, 1)
		assert.Equal(t, fallback, details.Items[0].ProviderModelFallback)
	}
}

func TestUpstreamSecondsUseFrozenUnitsAndSettlementFacts(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	facts := `"statement_snapshot":{"billing_mode":"per_second"},"usage_units":{"duration":"second"},"usage_facts":{"duration":6.5}`
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ChannelId: 81, CreatedAt: 1100, Type: LogTypeConsume, ModelName: "video", Other: `{` + facts + `,"task_id":"t","task_billing_event":"create"}`},
		{UserId: 7, ChannelId: 81, CreatedAt: 1101, Type: LogTypeRefund, ModelName: "video", Other: `{` + facts + `,"task_id":"t","task_billing_event":"adjustment"}`},
		{UserId: 7, ChannelId: 81, CreatedAt: 1102, Type: LogTypeRefund, ModelName: "video", Other: `{` + facts + `,"task_id":"t","task_billing_event":"refund"}`},
		{UserId: 7, ChannelId: 81, CreatedAt: 1103, Type: LogTypeConsume, ModelName: "zero", Other: `{"statement_snapshot":{"billing_mode":"per_second"},"usage_units":{"seconds":"second"},"usage_facts":{"seconds":0}}`},
		{UserId: 7, ChannelId: 81, CreatedAt: 1104, Type: LogTypeConsume, ModelName: "missing", Other: `{"statement_snapshot":{"billing_mode":"per_second"},"seconds":9}`},
	}).Error)
	details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{81}}, 1, 50, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 5)
	assert.Nil(t, details.Items[0].Seconds)
	require.NotNil(t, details.Items[1].Seconds)
	assert.Equal(t, "6.5", details.Items[1].Seconds.String())
	assert.Nil(t, details.Items[2].Seconds)
	require.NotNil(t, details.Items[3].Seconds)
	assert.Equal(t, "0", details.Items[3].Seconds.String())
	assert.Nil(t, details.Items[4].Seconds)
	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	require.NotNil(t, summary.Groups[0].Usage.Seconds)
	assert.Equal(t, "6.5", summary.Groups[0].Usage.Seconds.String())
	assert.EqualValues(t, 1, summary.Groups[0].Usage.SecondsUnavailableRows)
}
