package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const taskSixTierExpression = `param("_task.resolution") == "1080p"
  ? (param("_task.has_video_input") == true
      ? tier("1080p_video", c * 4.768115942)
      : tier("1080p", c * 7.811594203))
  : param("_task.resolution") == "4k"
    ? (param("_task.has_video_input") == true
        ? tier("4k_video", c * 2.434782609)
        : tier("4k", c * 4.057971014))
    : (param("_task.has_video_input") == true
        ? tier("480_720_video", c * 4.362318841)
        : tier("480_720", c * 7.101449275))`

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
		{resolution: "720p", hasVideo: true, tier: "480_720_video", rate: 4.362318841},
		{resolution: "720p", hasVideo: false, tier: "480_720", rate: 7.101449275},
		{resolution: "1080p", hasVideo: true, tier: "1080p_video", rate: 4.768115942},
		{resolution: "1080p", hasVideo: false, tier: "1080p", rate: 7.811594203},
		{resolution: "4k", hasVideo: true, tier: "4k_video", rate: 2.434782609},
		{resolution: "4k", hasVideo: false, tier: "4k", rate: 4.057971014},
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
