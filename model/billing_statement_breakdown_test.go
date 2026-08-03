package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_statement_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetUserBillingStatementBreakdownUsesSettledLogs(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		billing_statement_setting.ContextThresholdsOption: `{"model-a":1000,"claude-model":1000}`,
	}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			billing_statement_setting.ContextThresholdsOption: `{}`,
		}))
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})

	logs := []Log{
		{
			UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 11,
			TokenName: "old-key", ModelName: "model-a", PromptTokens: 700, Quota: 100,
			Other: `{"cache_tokens":100,"cache_write_tokens":200,"input_tokens_total":800,"billing_mode":"tiered_expr"}`,
		},
		{
			UserId: 7, CreatedAt: 1200, Type: LogTypeConsume, TokenId: 11,
			TokenName: "current-key", ModelName: "model-a", PromptTokens: 900, Quota: 200,
			Other: `{"cache_creation_tokens":300,"cache_creation_tokens_5m":200,"cache_creation_tokens_1h":200,"input_tokens_total":1200}`,
		},
		{
			UserId: 7, CreatedAt: 1250, Type: LogTypeConsume, TokenId: 11,
			TokenName: "current-key", ModelName: "model-a", Quota: 999,
			Other: `{"task_id":"async-adjustment","cache_tokens":999}`,
		},
		{
			UserId: 7, CreatedAt: 1275, Type: LogTypeConsume, TokenId: 11,
			TokenName: "current-key", ModelName: "model-a", Quota: 50,
			Other: `{}`,
		},
		{
			UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, TokenId: 12,
			TokenName: "other-key", ModelName: "model-b", PromptTokens: 2000, Quota: 300,
			Other: `{"cache_tokens":50,"input_tokens_total":2000}`,
		},
		{
			UserId: 7, CreatedAt: 1400, Type: LogTypeConsume, TokenId: 13,
			TokenName: "claude-key", ModelName: "claude-model", PromptTokens: 600, Quota: 400,
			Other: `{"usage_semantic":"anthropic","cache_tokens":300,"cache_write_tokens":200}`,
		},
		{
			UserId: 7, CreatedAt: 1450, Type: LogTypeConsume, TokenId: 14,
			TokenName: "damaged-key", ModelName: "damaged-model", PromptTokens: 500, Quota: 25,
			Other: `{"cache_tokens":100`,
		},
		{
			UserId: 8, CreatedAt: 1500, Type: LogTypeConsume, TokenId: 11,
			TokenName: "other-user", ModelName: "model-a", Quota: 1000,
			Other: `{"cache_tokens":1000,"input_tokens_total":2000}`,
		},
	}
	require.NoError(t, db.Create(&logs).Error)

	items, summary, err := GetUserBillingStatementBreakdown(7, 1000, 1600, 0, "")
	require.NoError(t, err)
	require.Len(t, items, 4)

	itemsByModel := make(map[string]BillingStatementBreakdownItem, len(items))
	for _, item := range items {
		itemsByModel[item.ModelName] = item
	}

	modelA := itemsByModel["model-a"]
	assert.Equal(t, "current-key", modelA.TokenName)
	assert.EqualValues(t, 3, modelA.Requests)
	assert.EqualValues(t, 1349, modelA.GrossQuota)
	assert.EqualValues(t, 999, modelA.UnallocatedAdjustmentQuota)
	require.NotNil(t, modelA.Cache)
	assert.EqualValues(t, 1, modelA.Cache.HitRequests)
	assert.EqualValues(t, 2, modelA.Cache.WriteRequests)
	assert.EqualValues(t, 3, modelA.Cache.DenominatorRequests)
	assert.Equal(t, "all_settled_requests", modelA.Cache.DenominatorScope)
	assert.EqualValues(t, 100, modelA.Cache.ReadTokens)
	assert.EqualValues(t, 600, modelA.Cache.WriteTokens)
	assert.EqualValues(t, 200, modelA.Cache.WriteTokens5m)
	assert.EqualValues(t, 200, modelA.Cache.WriteTokens1h)
	assert.InDelta(t, 1.0/3.0, modelA.Cache.HitRequestRatio, 0.0001)
	require.NotNil(t, modelA.Context)
	assert.EqualValues(t, 1000, modelA.Context.ThresholdTokens)
	assert.Equal(t, "current_model_config", modelA.Context.ThresholdSource)
	assert.EqualValues(t, 2, modelA.Context.ClassifiedRequests)
	assert.EqualValues(t, 1, modelA.Context.UnclassifiedRequests)
	assert.EqualValues(t, 1, modelA.Context.ShortRequests)
	assert.EqualValues(t, 1, modelA.Context.LongRequests)
	assert.EqualValues(t, 800, modelA.Context.ShortInputTokens)
	assert.EqualValues(t, 1200, modelA.Context.LongInputTokens)
	assert.EqualValues(t, 100, modelA.Context.ShortGrossQuota)
	assert.EqualValues(t, 200, modelA.Context.LongGrossQuota)
	assert.InDelta(t, 2.0/3.0, modelA.Context.ClassificationCoverage, 0.0001)
	require.NotNil(t, modelA.BillingMode)
	assert.EqualValues(t, 1, modelA.BillingMode.TieredRequests)
	assert.EqualValues(t, 100, modelA.BillingMode.TieredGrossQuota)

	modelB := itemsByModel["model-b"]
	assert.Nil(t, modelB.Context, "models without a configured threshold stay blank")
	require.NotNil(t, modelB.Cache)
	assert.EqualValues(t, 1, modelB.Cache.HitRequests)

	claude := itemsByModel["claude-model"]
	require.NotNil(t, claude.Context)
	assert.EqualValues(t, 1, claude.Context.LongRequests)
	assert.EqualValues(t, 1100, claude.Context.LongInputTokens)
	assert.EqualValues(t, 400, claude.Context.LongGrossQuota)

	damaged := itemsByModel["damaged-model"]
	assert.EqualValues(t, 1, damaged.Requests)
	assert.EqualValues(t, 25, damaged.GrossQuota)
	assert.Nil(t, damaged.Cache)
	assert.Nil(t, damaged.Context)
	assert.Nil(t, damaged.BillingMode)
	require.NotNil(t, damaged.DataQuality)
	assert.EqualValues(t, 1, damaged.DataQuality.UnavailableRequests)

	assert.EqualValues(t, 6, summary.Requests)
	assert.EqualValues(t, 2074, summary.GrossQuota)
	assert.EqualValues(t, 999, summary.UnallocatedAdjustmentQuota)
	require.NotNil(t, summary.Cache)
	assert.EqualValues(t, 3, summary.Cache.HitRequests)
	assert.EqualValues(t, 6, summary.Cache.DenominatorRequests)
	assert.Equal(t, "all_settled_requests", summary.Cache.DenominatorScope)
	assert.EqualValues(t, 450, summary.Cache.ReadTokens)
	assert.EqualValues(t, 200, summary.Cache.WriteTokens5m)
	assert.EqualValues(t, 200, summary.Cache.WriteTokens1h)
	assert.InDelta(t, 3.0/6.0, summary.Cache.HitRequestRatio, 0.0001)
	require.NotNil(t, summary.Context)
	assert.EqualValues(t, 3, summary.Context.ClassifiedRequests)
	assert.EqualValues(t, 1, summary.Context.UnclassifiedRequests)
	assert.EqualValues(t, 1, summary.Context.ShortRequests)
	assert.EqualValues(t, 2, summary.Context.LongRequests)
	assert.EqualValues(t, 800, summary.Context.ShortInputTokens)
	assert.EqualValues(t, 2300, summary.Context.LongInputTokens)
	assert.InDelta(t, 0.75, summary.Context.ClassificationCoverage, 0.0001)
	require.NotNil(t, summary.BillingMode)
	assert.EqualValues(t, 1, summary.BillingMode.TieredRequests)
	require.NotNil(t, summary.DataQuality)
	assert.EqualValues(t, 1, summary.DataQuality.UnavailableRequests)

	statementItems, statementSummary, err := GetUserBillingStatement(7, 1000, 1600, 0, "")
	require.NoError(t, err)
	statementByModel := make(map[string]BillingStatementItem, len(statementItems))
	for _, item := range statementItems {
		statementByModel[item.ModelName] = item
	}
	assert.Equal(t, statementByModel["model-a"].GrossQuota, modelA.GrossQuota)
	assert.Equal(t, statementSummary.GrossQuota, summary.GrossQuota)
}

func TestBillingBreakdownInputTokensRequiresReliableSemantic(t *testing.T) {
	var other map[string]json.RawMessage
	require.NoError(t, common.UnmarshalJsonStr(`{"claude":true,"cache_tokens":100}`, &other))

	tokens, ok := billingBreakdownInputTokens(
		billingStatementBreakdownLog{PromptTokens: 500},
		other,
		100,
		0,
	)
	assert.False(t, ok, "request format alone must not be treated as Anthropic billing semantics")
	assert.Zero(t, tokens)
}

func TestBillingStatementBreakdownNewTotalsSaturate(t *testing.T) {
	context := BillingStatementContextBreakdown{
		ClassifiedRequests:   math.MaxInt64,
		UnclassifiedRequests: 1,
		LongInputTokens:      math.MaxInt64 - 1,
	}
	accumulateBillingStatementContext(&context, 10, 1, 1)
	finalizeBillingStatementContext(&context)

	assert.EqualValues(t, math.MaxInt64, context.ClassifiedRequests)
	assert.EqualValues(t, math.MaxInt64, context.LongInputTokens)
	assert.InDelta(t, 1, context.ClassificationCoverage, 0.0001)
}

func TestBillingStatementTotalsReportSaturation(t *testing.T) {
	var summary BillingStatementSummary
	summary.GrossQuota = math.MaxInt64
	if addBillingStatementValue(&summary.GrossQuota, 1) {
		markBillingStatementSaturated(&summary.DataQuality)
	}

	assert.EqualValues(t, math.MaxInt64, summary.GrossQuota)
	require.NotNil(t, summary.DataQuality)
	assert.True(t, summary.DataQuality.Saturated)
}
