package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingReconciliationEmptyCollectionsEncodeAsArrays(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 6, Username: "empty", Quota: 100}).Error)

	statement, err := GetBillingCustomerStatement(6, 1000, 1500, "api_key", 0, "", "")
	require.NoError(t, err)
	require.NotNil(t, statement.Groups)
	statementJSON, err := common.Marshal(statement)
	require.NoError(t, err)
	assert.Contains(t, string(statementJSON), `"groups":[]`)

	summary, err := GetProviderBillingSummary(1000, 1500, 1000, 0, "", "", 1)
	require.NoError(t, err)
	require.NotNil(t, summary.Channels)
	summaryJSON, err := common.Marshal(summary)
	require.NoError(t, err)
	assert.Contains(t, string(summaryJSON), `"channels":[]`)
}

func setupBillingReconciliationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &Log{}))
	require.NoError(t, migrateBillingReconciliationDB())
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
	return db
}

func TestBillingCustomerStatementAggregatesByDimensionAndBillingMode(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer", Quota: 8800}).Error)
	require.NoError(t, db.Create(&[]Channel{{Id: 21, Name: "primary"}, {Id: 22, Name: "secondary"}}).Error)
	logs := []Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "token-model", PromptTokens: 100, CompletionTokens: 20, Quota: 1200, Other: `{"group_ratio":0.8,"model_ratio":1,"cache_tokens":5,"cache_creation_tokens":3}`},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeConsume, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "call-model", Quota: 400, Other: `{"group_ratio":1,"model_price":0.002}`},
		{UserId: 7, CreatedAt: 1250, Type: LogTypeRefund, TokenId: 11, TokenName: "key-a", ChannelId: 21, ModelName: "call-model", Quota: 400, Other: `{"group_ratio":1,"model_price":0.002}`},
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, TokenId: 12, TokenName: "key-b", ChannelId: 22, ModelName: "token-model", PromptTokens: 50, CompletionTokens: 10, Quota: 600, Other: `{"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 8, CreatedAt: 1300, Type: LogTypeConsume, TokenId: 11, TokenName: "other", ChannelId: 21, ModelName: "token-model", PromptTokens: 999, Quota: 999, Other: `{"group_ratio":1,"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	statement, err := GetBillingCustomerStatement(7, 1000, 1500, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 2)
	assert.Equal(t, 8800, statement.CurrentBalance)
	assert.EqualValues(t, 3, statement.Summary.Requests)
	assert.EqualValues(t, 2200, statement.Summary.GrossQuota)
	assert.EqualValues(t, 400, statement.Summary.RefundQuota)
	assert.EqualValues(t, 1800, statement.Summary.NetQuota)

	models := make(map[string]BillingReconciliationModelSummary)
	for _, group := range statement.Groups {
		for _, item := range group.Models {
			models[fmt.Sprintf("%d:%s:%s", group.Id, item.ModelName, item.BillingMode)] = item
		}
	}
	tokenItem := models["11:token-model:token"]
	assert.EqualValues(t, 100, tokenItem.Usage.InputTokens)
	assert.EqualValues(t, 5, tokenItem.Usage.CacheReadTokens)
	assert.EqualValues(t, 3, tokenItem.Usage.CacheWriteTokens)
	assert.NotNil(t, tokenItem.OriginalQuota)
	assert.EqualValues(t, 1500, *tokenItem.OriginalQuota)
	assert.NotNil(t, tokenItem.DiscountRatio)
	assert.InDelta(t, 0.8, *tokenItem.DiscountRatio, 0.000001)
	assert.Equal(t, 11, tokenItem.DetailFilter.TokenId)
	callItem := models["11:call-model:per_call"]
	assert.EqualValues(t, 1, callItem.Usage.BillableCalls)
	assert.EqualValues(t, 1, callItem.Usage.RefundedCalls)
	assert.Zero(t, callItem.Usage.NetQuota)

	channelStatement, err := GetBillingCustomerStatement(7, 1000, 1500, "channel", 21, "", "")
	require.NoError(t, err)
	require.Len(t, channelStatement.Groups, 1)
	assert.Equal(t, "primary", channelStatement.Groups[0].Name)
	assert.EqualValues(t, 2, channelStatement.Groups[0].Usage.Requests)
}

func TestBillingCustomerStatementDoesNotExposePartialOriginalQuota(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 17, Username: "partial", Quota: 1000}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 17, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 11, TokenName: "key", ChannelId: 21, ModelName: "model", PromptTokens: 10, Quota: 800, Other: `{"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 17, CreatedAt: 1200, Type: LogTypeConsume, TokenId: 11, TokenName: "key", ChannelId: 21, ModelName: "model", PromptTokens: 10, Quota: 500, Other: `{"model_ratio":1}`},
	}).Error)

	statement, err := GetBillingCustomerStatement(17, 1000, 1500, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	item := statement.Groups[0].Models[0]
	assert.Nil(t, item.OriginalQuota)
	assert.Nil(t, statement.Groups[0].OriginalQuota)
	assert.Nil(t, statement.OriginalQuota)
	require.NotNil(t, statement.DataQuality)
	assert.Equal(t, "partial", statement.DataQuality.Status)
	assert.EqualValues(t, 1, statement.DataQuality.MissingHistoricalPriceRows)
}

func TestBillingReconciliationUsesFrozenSnapshotPricingBeforeLegacyTopLevelFacts(t *testing.T) {
	parsed := parseBillingReconciliationLog(billingReconciliationLog{
		PromptTokens: 10,
		Other:        `{"group_ratio":0.5,"admin_info":{"statement_snapshot":{"snapshot_version":1,"billing_mode":"token","provider_model":"provider-model","group_ratio":0.8,"model_ratio":2}}}`,
	})

	require.NotNil(t, parsed.discountRatio)
	assert.InDelta(t, 0.8, *parsed.discountRatio, 0.000001)
	assert.Equal(t, "provider-model", parsed.providerModel)
	assert.Equal(t, BillingReconciliationModeToken, parsed.billingMode)
}

func TestBillingReconciliationSeparatesContractDiscountFromGroupRatio(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 18, Username: "contract", Quota: 10000}).Error)
	require.NoError(t, db.Create(&Log{
		UserId: 18, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 12, TokenName: "key", ChannelId: 21,
		ModelName: "model", PromptTokens: 10, Quota: 400,
		Other: `{"group_ratio":0.8,"contract_discount":"0.5","model_ratio":1}`,
	}).Error)

	statement, err := GetBillingCustomerStatement(18, 1000, 1500, "api_key", 0, "", "")
	require.NoError(t, err)
	item := statement.Groups[0].Models[0]
	require.NotNil(t, item.OriginalQuota)
	assert.EqualValues(t, 1000, *item.OriginalQuota)
	require.NotNil(t, item.DiscountRatio)
	assert.InDelta(t, 0.8, *item.DiscountRatio, 0.000001)
	require.NotNil(t, item.ContractDiscountRatio)
	assert.InDelta(t, 0.5, *item.ContractDiscountRatio, 0.000001)
}

func TestBillingReconciliationClassifiesLegacyBillingFactsWithoutUsage(t *testing.T) {
	tests := []struct {
		name string
		log  billingReconciliationLog
		mode string
	}{
		{
			name: "text ratio is token billing even when a failed request has no usage",
			log:  billingReconciliationLog{Other: `{"request_path":"/v1/responses","model_price":-1,"model_ratio":2.5}`},
			mode: BillingReconciliationModeToken,
		},
		{
			name: "task marker is per call even without a fixed model price",
			log:  billingReconciliationLog{Other: `{"request_path":"/v1/video/generations","is_task":true,"model_price":0}`},
			mode: BillingReconciliationModePerCall,
		},
		{
			name: "legacy midjourney route stays per call when it used ratio fallback",
			log:  billingReconciliationLog{Other: `{"request_path":"/mj/submit/imagine","model_price":-1,"model_ratio":1}`},
			mode: BillingReconciliationModePerCall,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := parseBillingReconciliationLog(test.log)
			assert.Equal(t, test.mode, parsed.billingMode)
		})
	}
}

func TestProviderBillingSummaryUsesUnmappedCustomerModelAsExactProviderIdentity(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 52, Name: "legacy-provider"}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 52, ModelName: "gpt-5.6-sol", PromptTokens: 10, Other: `{"model_price":-1,"model_ratio":2.5}`},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeConsume, ChannelId: 52, ModelName: "gpt-5.6-sol", Other: `{"request_path":"/v1/responses","model_price":-1,"model_ratio":2.5}`},
	}).Error)

	summary, err := GetProviderBillingSummary(1000, 1500, 1000, 52, "", "", 9)
	require.NoError(t, err)
	require.Len(t, summary.Channels, 1)
	require.Len(t, summary.Channels[0].Models, 1)
	item := summary.Channels[0].Models[0]
	assert.Equal(t, "gpt-5.6-sol", item.ProviderModel)
	assert.False(t, item.ProviderModelFallback)
	assert.Equal(t, BillingReconciliationModeToken, item.BillingMode)
	assert.Zero(t, item.DataQuality.ProviderModelFallbackRows)
	encoded, err := common.Marshal(summary)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"provider_usage"`)
	assert.NotContains(t, string(encoded), `"usage_comparison"`)
	assert.NotContains(t, string(encoded), `"provider_amount"`)
	assert.NotContains(t, string(encoded), `"review"`)
	assert.NotContains(t, string(encoded), `"gross_quota"`)
	assert.NotContains(t, string(encoded), `"refund_quota"`)
	assert.NotContains(t, string(encoded), `"refunded_calls"`)
}

func TestProviderBillingSummaryMarksOnlyBrokenMappedIdentityAsFallback(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 53, Name: "legacy-provider"}).Error)
	require.NoError(t, db.Create(&Log{
		UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 53, ModelName: "customer-model", PromptTokens: 10,
		Other: `{"is_model_mapped":true,"model_ratio":1}`,
	}).Error)

	summary, err := GetProviderBillingSummary(1000, 1500, 1000, 53, "", "", 9)
	require.NoError(t, err)
	require.Len(t, summary.Channels, 1)
	require.Len(t, summary.Channels[0].Models, 1)
	item := summary.Channels[0].Models[0]
	assert.Equal(t, "customer-model", item.ProviderModel)
	assert.True(t, item.ProviderModelFallback)
	require.NotNil(t, item.DataQuality)
	assert.EqualValues(t, 1, item.DataQuality.ProviderModelFallbackRows)
}

func TestUsageLogDetailFilterUsesStableTokenId(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 27, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 101, TokenName: "duplicate-name", ModelName: "model"},
		{UserId: 27, CreatedAt: 1200, Type: LogTypeConsume, TokenId: 102, TokenName: "duplicate-name", ModelName: "model"},
	}).Error)

	logs, total, err := GetUserLogs(27, LogTypeUnknown, 1000, 1500, "model", "", 101, 0, 10, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, 101, logs[0].TokenId)
}

func TestProviderBillingDiscountInheritanceVersioningAndAudit(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	july := time.Date(2026, time.July, 1, 0, 0, 0, 0, location).Unix()
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, location).Unix()
	require.NoError(t, db.Create(&Channel{Id: 31, Name: "provider"}).Error)

	previous := &ProviderBillingDiscount{PeriodStart: july, ChannelId: 31, ProviderModel: "model-a", BillingMode: BillingReconciliationModeToken, Discount: decimal.RequireFromString("0.85"), Reason: "supplier contract"}
	require.NoError(t, SaveProviderBillingDiscount(previous, 0, 9))
	items := map[providerBillingSummaryKey]*ProviderBillingPlatformSummary{
		{channelId: 31, model: "model-a", mode: BillingReconciliationModeToken}: {},
	}
	projections, err := getProviderBillingDiscountProjections(august, items, 10)
	require.NoError(t, err)
	projection := projections[providerBillingSummaryKey{channelId: 31, model: "model-a", mode: BillingReconciliationModeToken}]
	assert.True(t, projection.Value.Equal(decimal.RequireFromString("0.85")))
	assert.Equal(t, "previous_period", projection.Source)
	assert.Equal(t, july, projection.SourcePeriod)
	assert.EqualValues(t, 1, projection.Version)

	var current ProviderBillingDiscount
	require.NoError(t, db.Where("period_start = ? AND channel_id = ? AND provider_model = ? AND billing_mode = ?", august, 31, "model-a", BillingReconciliationModeToken).First(&current).Error)
	assert.Equal(t, july, current.CopiedFromPeriod)
	previous.Discount = decimal.RequireFromString("0.7")
	previous.Reason = "late July correction"
	require.NoError(t, SaveProviderBillingDiscount(previous, 1, 9))
	projectionAfterCorrection, err := getProviderBillingDiscountProjections(august, items, 10)
	require.NoError(t, err)
	assert.True(t, projectionAfterCorrection[providerBillingSummaryKey{channelId: 31, model: "model-a", mode: BillingReconciliationModeToken}].Value.Equal(decimal.RequireFromString("0.85")))

	current.Discount = decimal.RequireFromString("0.8")
	current.Reason = "August adjustment"
	require.ErrorIs(t, SaveProviderBillingDiscount(&current, 0, 10), ErrBillingReconciliationVersionConflict)
	require.NoError(t, SaveProviderBillingDiscount(&current, 1, 10))
	assert.EqualValues(t, 2, current.Version)

	var audits []ProviderBillingAudit
	require.NoError(t, db.Where("entity_type = ?", "discount").Order("id").Find(&audits).Error)
	require.Len(t, audits, 4)
	assert.Equal(t, "create", audits[0].Action)
	assert.Equal(t, "create", audits[1].Action)
	assert.Equal(t, "update", audits[3].Action)
	assert.Contains(t, audits[3].Before, `"discount":"0.85"`)
	assert.Contains(t, audits[3].After, `"discount":"0.8"`)
}

func TestProviderBillingDetailDoesNotNarrowMergedProviderModelToOneCustomerModel(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 51, Name: "provider"}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 51, ModelName: "customer-a", PromptTokens: 10, Other: `{"admin_info":{"statement_snapshot":{"billing_mode":"token","provider_model":"provider-model"}}}`},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeConsume, ChannelId: 51, ModelName: "customer-b", PromptTokens: 20, Other: `{"admin_info":{"statement_snapshot":{"billing_mode":"token","provider_model":"provider-model"}}}`},
	}).Error)

	summary, err := GetProviderBillingSummary(1000, 1500, 1000, 51, "", "", 9)
	require.NoError(t, err)
	require.Len(t, summary.Channels, 1)
	require.Len(t, summary.Channels[0].Models, 1)
	assert.Empty(t, summary.Channels[0].Models[0].DetailFilter.ModelName)
}
