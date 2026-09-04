package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contractBillingFact() *hosttypes.ContractBillingFact {
	return &hosttypes.ContractBillingFact{
		UserId: 10, ContractVersion: 4, PublicModel: "contract-model",
		RouteGroup: "contract-group", RatioUnits: 80_000_000,
	}
}

func TestCustomerContractTextQuotaDiscountsModelButNotToolSurcharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operation_setting.SetToolPriceForTest("contract_tool", 0.2)
	t.Cleanup(func() { operation_setting.DeleteToolPriceForTest("contract_tool") })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		OriginModelName: "contract-model", ContractBillingFact: contractBillingFact(), StartTime: time.Now(),
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
			"contract_tool": {ToolName: "contract_tool", CallCount: 1},
		}},
		PriceData: hosttypes.PriceData{
			ModelRatio: 1, CompletionRatio: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 0.87},
		},
	}

	summary := calculateTextQuotaSummary(ctx, info, &dto.Usage{PromptTokens: 100, TotalTokens: 100})

	// Model: 100 × 0.87 × 0.8 = 69.6 -> 70. Tool: 0.2/1000 × 500000 × 0.87 = 87.
	assert.True(t, decimal.NewFromInt(87).Equal(summary.ToolCallSurchargeQuota))
	assert.Equal(t, 157, summary.Quota)
}

func TestCustomerContractFixedAndRealtimeAudioQuotaUseFrozenDiscount(t *testing.T) {
	fact := contractBillingFact()
	fixed, clamp, err := calculateAudioQuota(QuotaInfo{
		UsePrice: true, ModelPrice: 0.0002, GroupRatio: 0.87, ContractFact: fact,
	})
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 70, fixed)

	token, clamp, err := calculateAudioQuota(QuotaInfo{
		InputDetails: TokenDetails{TextTokens: 100}, ModelName: "contract-model",
		ModelRatio: 1, GroupRatio: 0.87, ContractFact: fact,
	})
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 70, token)
}

func TestFinalAudioSettlementCarriesFixedModelPriceIntoQuotaCalculation(t *testing.T) {
	fact := contractBillingFact()
	info := finalAudioQuotaInfo(TokenDetails{}, TokenDetails{}, "contract-model", true, 0.0002, 9, 0.87, fact)

	quota, clamp, err := calculateAudioQuota(info)

	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 70, quota)
}

func TestCustomerContractTieredSettlementAppliesBeforeFinalRounding(t *testing.T) {
	expression := `tier("base", p * 2)`
	info := &relaycommon.RelayInfo{
		ContractBillingFact: contractBillingFact(),
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
			GroupRatio: 0.87, EstimatedQuotaAfterGroup: 70, QuotaPerUnit: common.QuotaPerUnit,
		},
		FinalPreConsumedQuota: 70,
	}

	ok, quota, result := TryTieredSettle(info, billingexpr.TokenParams{P: 100})
	require.True(t, ok)
	require.NotNil(t, result)
	// raw quota 100, then group 0.87 and contract 0.8 => 69.6, rounded once.
	assert.Equal(t, 70, quota)
}

func TestCustomerContractAsyncTokenSettlementUsesFrozenPricingFacts(t *testing.T) {
	task := &model.Task{
		TaskID: "task-contract-frozen", UserId: 10, Group: "changed-current-group",
		Properties: model.Properties{OriginModelName: "contract-model"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginModelName: "contract-model", ModelRatio: 2, GroupRatio: 0.87,
			OtherRatios: map[string]float64{"duration": 1.5}, ContractFact: contractBillingFact(),
		}},
	}

	quota, clamp, reason, ok := calculateTaskQuotaByTokens(task, 100)
	require.True(t, ok)
	assert.Nil(t, clamp)
	assert.NotEmpty(t, reason)
	// 100 × 2 × 0.87 × 1.5 × 0.8 = 208.8, rounded once.
	assert.Equal(t, 209, quota)
}

func TestNativeAsyncTokenSettlementKeepsLegacyCurrentPricingLookup(t *testing.T) {
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousSpecialGroups := ratio_setting.GroupGroupRatio2JSONString()
	previousModels := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"native-current":0.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"native-current":{"native-current":0.25}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"native-model":2}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousSpecialGroups))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModels))
	})

	task := &model.Task{
		TaskID: "task-native-current", Group: "native-current",
		Properties: model.Properties{OriginModelName: "native-model"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			ModelRatio: 9, GroupRatio: 9, OtherRatios: map[string]float64{"duration": 1.5},
		}},
	}

	quota, clamp, _, ok := calculateTaskQuotaByTokens(task, 100)
	require.True(t, ok)
	assert.Nil(t, clamp)
	assert.Equal(t, 75, quota, "native tasks must retain their special self-group ratio override")
}

func TestFirstAsyncSettlementUsesPersistedNormalizedUsage(t *testing.T) {
	truncate(t)
	const userID = 8120
	seedUser(t, userID, 1000)
	task := persistedAsyncTask(t, userID, 1000, model.TaskStatusSuccess)
	task.PrivateData.BillingContext.ModelRatio = 1
	task.PrivateData.BillingContext.GroupRatio = 1
	task.PrivateData.BillingContext.ContractFact = contractBillingFact()
	task.PrivateData.AsyncBilling.ActualUsageReported = true
	task.PrivateData.AsyncBilling.ActualTokens = 40
	require.NoError(t, task.UpdateBilling())

	require.True(t, settleTaskBillingWithState(context.Background(), &mockAdaptor{}, task, &relaycommon.TaskInfo{TotalTokens: 900}))

	settled := reloadTask(t, task.ID)
	assert.Equal(t, 32, settled.Quota, "40 normalized tokens × 0.8 contract ratio must win over raw total_tokens")
}

func TestNativeTieredSettlementRemainsByteForByteOnExistingResult(t *testing.T) {
	expression := `tier("base", p * 2)`
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
		GroupRatio: 0.87, EstimatedQuotaAfterGroup: 87, QuotaPerUnit: common.QuotaPerUnit,
	}
	params := billingexpr.TokenParams{P: 100}
	want, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, params, billingexpr.RequestInput{})
	require.NoError(t, err)

	ok, quota, got := TryTieredSettle(&relaycommon.RelayInfo{TieredBillingSnapshot: snapshot}, params)
	require.True(t, ok)
	require.NotNil(t, got)
	assert.Equal(t, want.ActualQuotaAfterGroup, quota)
	assert.Equal(t, want.ActualQuotaAfterGroup, got.ActualQuotaAfterGroup)
}

func TestCustomerContractBillingMetadataDoesNotExposeRouteGroup(t *testing.T) {
	other := model.NewLogOther()
	appendCustomerContractBillingInfo(other, contractBillingFact())
	public := other.Snapshot()
	assert.EqualValues(t, 4, public["contract_version"])
	assert.Equal(t, "0.8", public["contract_discount"])
	assert.NotContains(t, public, "route_group")
}
