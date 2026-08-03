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
      ? tier("1080p_video", c * 4.7)
      : tier("1080p", c * 7.7))
  : param("_task.resolution") == "4k"
    ? (param("_task.has_video_input") == true
        ? tier("4k_video", c * 2.4)
        : tier("4k", c * 4.0))
    : (param("_task.has_video_input") == true
        ? tier("480p720p_video", c * 4.3)
        : tier("480p720p", c * 7.0))`

const funCloudStandardListPriceExpression = `param("_task.has_video_input") == true
  ? (param("_task.resolution") == "1080p"
      ? tier("1080p_video", param("_task.duration_seconds") * 398900)
      : param("_task.resolution") == "720p"
        ? tier("720p_video", param("_task.duration_seconds") * 160000)
        : tier("480p_video", param("_task.duration_seconds") * 146500))
  : (param("_task.resolution") == "1080p"
      ? tier("1080p", param("_task.duration_seconds") * 365100)
      : param("_task.resolution") == "720p"
        ? tier("720p", param("_task.duration_seconds") * 146500)
        : tier("480p", param("_task.duration_seconds") * 67900))`

const funCloudFastListPriceExpression = `param("_task.has_video_input") == true
  ? (param("_task.resolution") == "720p"
      ? tier("720p_video", param("_task.duration_seconds") * 160000)
      : tier("480p_video", param("_task.duration_seconds") * 146500))
  : (param("_task.resolution") == "720p"
      ? tier("720p", param("_task.duration_seconds") * 146500)
      : tier("480p", param("_task.duration_seconds") * 67900))`

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

func TestModelPriceHelperTaskTieredMatchesFunCloudListPrices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const standardModel = "seedance-2.0-standard"
	const fastModel = "seedance-2.0-fast"
	loadTaskPricingConfig(t, map[string]string{
		standardModel: funCloudStandardListPriceExpression,
		fastModel:     funCloudFastListPriceExpression,
	}, map[string]int{standardModel: 1, fastModel: 1})

	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
		tier       string
		usdPerSec  float64
	}{
		{name: "standard 480p", model: standardModel, resolution: "480p", tier: "480p", usdPerSec: 0.0679},
		{name: "standard 720p", model: standardModel, resolution: "720p", tier: "720p", usdPerSec: 0.1465},
		{name: "standard 1080p", model: standardModel, resolution: "1080p", tier: "1080p", usdPerSec: 0.3651},
		{name: "standard v2v 480p", model: standardModel, resolution: "480p", hasVideo: true, tier: "480p_video", usdPerSec: 0.1465},
		{name: "standard v2v 720p", model: standardModel, resolution: "720p", hasVideo: true, tier: "720p_video", usdPerSec: 0.16},
		{name: "standard v2v 1080p", model: standardModel, resolution: "1080p", hasVideo: true, tier: "1080p_video", usdPerSec: 0.3989},
		{name: "fast 480p", model: fastModel, resolution: "480p", tier: "480p", usdPerSec: 0.0679},
		{name: "fast 720p", model: fastModel, resolution: "720p", tier: "720p", usdPerSec: 0.1465},
		{name: "fast v2v 480p", model: fastModel, resolution: "480p", hasVideo: true, tier: "480p_video", usdPerSec: 0.1465},
		{name: "fast v2v 720p", model: fastModel, resolution: "720p", hasVideo: true, tier: "720p_video", usdPerSec: 0.16},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const duration = 10
			info := &relaycommon.RelayInfo{OriginModelName: test.model, UserGroup: "default", UsingGroup: "default"}
			price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{
				"resolution": test.resolution, "has_video_input": test.hasVideo, "duration_seconds": duration,
			})

			require.NoError(t, err)
			expected, clamp := common.QuotaRoundChecked(test.usdPerSec * duration * common.QuotaPerUnit)
			require.Nil(t, clamp)
			assert.Equal(t, expected, price.Quota)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, test.tier, info.TieredBillingSnapshot.EstimatedTier)
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
