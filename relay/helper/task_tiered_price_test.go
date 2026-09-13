package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const taskSixTierExpression = `param("_task.resolution") == "1080p"
  ? (param("_task.has_video_input") == true
      ? tier("1080p_video", c * 4.7)
      : tier("1080p", c * 7.7))
  : param("_task.resolution") == "4k"
    ? (param("_task.has_video_input") == true
        ? tier("4k_video", c * 2.4)
        : tier("4k", c * 4.0))
    : (param("_task.has_video_input") == true
        ? tier("480p720p_video", c * 4.3)
        : tier("480p720p", c * 7.0))`

type fixedTaskProbe map[string]any

func (p fixedTaskProbe) BuildTaskBillingProbe(*gin.Context, *relaycommon.RelayInfo) (map[string]any, error) {
	return p, nil
}

func loadTaskPricingConfig(t *testing.T, expressions map[string]string, estimates map[string]int) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })

	modes := make(map[string]string, len(expressions))
	for model := range expressions {
		modes[model] = "tiered_expr"
	}
	modeJSON, err := common.Marshal(modes)
	require.NoError(t, err)
	expressionJSON, err := common.Marshal(expressions)
	require.NoError(t, err)
	estimateJSON, err := common.Marshal(estimates)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":           string(modeJSON),
		"billing_setting.billing_expr":           string(expressionJSON),
		"task_billing_setting.preconsume_tokens": string(estimateJSON),
		"group_ratio_setting.group_ratio":        `{"default":1}`,
	}))
}

func taskPriceContext() *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	context.Set("group", "default")
	return context
}

func TestModelPriceHelperTaskTieredMatchesSixPriceTiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const model = "external-public-model"
	const estimatedTokens = 100000
	loadTaskPricingConfig(t, map[string]string{model: taskSixTierExpression}, map[string]int{model: estimatedTokens})

	tests := []struct {
		resolution string
		hasVideo   bool
		tier       string
		rate       float64
	}{
		{resolution: "480p", hasVideo: true, tier: "480p720p_video", rate: 4.3},
		{resolution: "480p", hasVideo: false, tier: "480p720p", rate: 7.0},
		{resolution: "720p", hasVideo: true, tier: "480p720p_video", rate: 4.3},
		{resolution: "720p", hasVideo: false, tier: "480p720p", rate: 7.0},
		{resolution: "1080p", hasVideo: true, tier: "1080p_video", rate: 4.7},
		{resolution: "1080p", hasVideo: false, tier: "1080p", rate: 7.7},
		{resolution: "4k", hasVideo: true, tier: "4k_video", rate: 2.4},
		{resolution: "4k", hasVideo: false, tier: "4k", rate: 4.0},
	}

	for _, test := range tests {
		t.Run(test.tier, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				OriginModelName: model,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "mapped-upstream-model",
					IsModelMapped:     true,
				},
				UserGroup:  "default",
				UsingGroup: "default",
			}
			price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{
				"resolution": test.resolution, "has_video_input": test.hasVideo,
			})

			require.NoError(t, err)
			expected, clamp := common.QuotaRoundChecked(float64(estimatedTokens) * test.rate / 1_000_000 * common.QuotaPerUnit)
			require.Nil(t, clamp)
			assert.Equal(t, expected, price.Quota)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, model, info.TieredBillingSnapshot.ModelName)
			assert.Equal(t, test.tier, info.TieredBillingSnapshot.EstimatedTier)
			assert.Nil(t, price.OtherRatios())
		})
	}
}

func TestModelPriceHelperTaskTieredRejectsMissingConfigAndOverflow(t *testing.T) {
	const missingExpression = "missing-expression"
	const missingEstimate = "missing-estimate"
	const overflowModel = "overflow-task"
	loadTaskPricingConfig(t, map[string]string{
		missingEstimate: `tier("base", c)`,
		overflowModel:   `tier("overflow", c * 1000000000000000)`,
	}, map[string]int{
		missingExpression: 100,
		overflowModel:     1000000,
	})

	baseInfo := func(model string) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{OriginModelName: model, UserGroup: "default", UsingGroup: "default"}
	}
	_, err := ModelPriceHelperTaskTiered(taskPriceContext(), baseInfo(missingExpression), fixedTaskProbe{})
	require.ErrorContains(t, err, "has not been priced")

	_, err = ModelPriceHelperTaskTiered(taskPriceContext(), baseInfo(missingEstimate), fixedTaskProbe{})
	require.ErrorContains(t, err, "pre-consume token upper bound is not configured")

	_, err = ModelPriceHelperTaskTiered(taskPriceContext(), baseInfo(overflowModel), fixedTaskProbe{})
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	assert.Equal(t, "QuotaRound", clamp.Op)
}

// TestModelPriceHelperTaskTieredRejectsNondeterministicExpr 验证异步任务 tiered 表达式
// 禁止 header()/hour() 等非确定性函数（P1-B）：预扣与结算两次求值上下文不同会导致价格不一致。
func TestModelPriceHelperTaskTieredRejectsNondeterministicExpr(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const hourModel = "nd-hour-task"
	const headerModel = "nd-header-task"
	loadTaskPricingConfig(t, map[string]string{
		hourModel:   `tier("base", c) * (hour("UTC") >= 0 ? 1 : 1)`,
		headerModel: `tier("base", c) * (has(header("x-tier"), "fast") ? 2 : 1)`,
	}, map[string]int{hourModel: 100, headerModel: 100})

	baseInfo := func(model string) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{OriginModelName: model, UserGroup: "default", UsingGroup: "default"}
	}
	_, err := ModelPriceHelperTaskTiered(taskPriceContext(), baseInfo(hourModel), fixedTaskProbe{})
	require.ErrorContains(t, err, "non-deterministic")

	_, err = ModelPriceHelperTaskTiered(taskPriceContext(), baseInfo(headerModel), fixedTaskProbe{})
	require.ErrorContains(t, err, "non-deterministic")
}

// TestModelPriceHelperTaskTieredSeedanceUsageExpression 覆盖迁移后的 Seedance
// u() 预扣路径：美元求值、受控 facts 冻结、预算缺失 fail-closed。
func TestModelPriceHelperTaskTieredSeedanceUsageExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const usageModel = "seedance-usage-model"
	const frozenModel = "seedance-frozen-model"
	const legacyModel = "seedance-legacy-model"
	loadTaskPricingConfig(t, map[string]string{
		usageModel:  `tier("base", u("tokens") * 5 / 1000000)`,
		frozenModel: `u("resolution") == "4k" ? tier("4k", 0.7) : tier("base", 0.5)`,
		legacyModel: taskSixTierExpression,
	}, map[string]int{
		usageModel:  300000,
		frozenModel: 0,
		legacyModel: 100000,
	})

	seedanceInfo := func(model string) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			OriginModelName: model,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeSeedanceLink,
				ChannelOtherSettings: dto.ChannelOtherSettings{
					VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
				},
			},
			UserGroup:  "default",
			UsingGroup: "default",
		}
	}

	t.Run("usd evaluation freezes controlled facts and units", func(t *testing.T) {
		info := seedanceInfo(usageModel)
		price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{"resolution": "1080p"})
		require.NoError(t, err)
		// 300000 tokens × $5/1M = $1.50 → 1.5 × QuotaPerUnit。
		expected, clamp := common.QuotaRoundChecked(1.5 * common.QuotaPerUnit)
		require.Nil(t, clamp)
		assert.Equal(t, expected, price.Quota)
		snap := info.TieredBillingSnapshot
		require.NotNil(t, snap)
		assert.True(t, snap.TaskUsageBilling)
		assert.Equal(t, float64(300000), snap.UsageFacts["tokens"])
		assert.Equal(t, "1080p", snap.UsageFacts["resolution"])
		assert.Equal(t, "token", snap.UsageUnits["tokens"])
		assert.Equal(t, "enum", snap.UsageUnits["resolution"])
	})

	t.Run("missing budget fails closed for a measured dependency", func(t *testing.T) {
		loadTaskPricingConfig(t, map[string]string{
			"seedance-unbudgeted": `tier("base", u("tokens") * 5 / 1000000)`,
		}, map[string]int{})
		_, err := ModelPriceHelperTaskTiered(taskPriceContext(), seedanceInfo("seedance-unbudgeted"), fixedTaskProbe{"resolution": "1080p"})
		require.ErrorContains(t, err, "pre-consume token upper bound is not configured")
	})

	t.Run("pure frozen conditions settle without a budget on the registered protocols", func(t *testing.T) {
		info := seedanceInfo(frozenModel)
		info.ChannelMeta.ChannelOtherSettings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolSynlinkVideoV1
		price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{"resolution": "4k"})
		require.NoError(t, err)
		expected, clamp := common.QuotaRoundChecked(0.7 * common.QuotaPerUnit)
		require.Nil(t, clamp)
		assert.Equal(t, expected, price.Quota)
	})

	t.Run("usage expressions are rejected outside the Seedance Link contract", func(t *testing.T) {
		native := &relaycommon.RelayInfo{
			OriginModelName: usageModel,
			ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDoubaoVideo},
			UserGroup:       "default",
			UsingGroup:      "default",
		}
		_, err := ModelPriceHelperTaskTiered(taskPriceContext(), native, fixedTaskProbe{"resolution": "1080p"})
		require.ErrorContains(t, err, "Seedance Link channel contract")
	})

	t.Run("legacy Seedance prices cannot accept new tasks", func(t *testing.T) {
		_, err := ModelPriceHelperTaskTiered(taskPriceContext(), seedanceInfo(legacyModel), fixedTaskProbe{"resolution": "1080p", "has_video_input": false})
		require.ErrorContains(t, err, "declared u() fields")
	})
}
