package billing_setting

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTaskPreConsumeTokensJSON(t *testing.T) {
	require.NoError(t, ValidateTaskPreConsumeTokensJSON(`{"external-model":250000}`))
	require.NoError(t, ValidateTaskPreConsumeTokensJSON(fmt.Sprintf(`{"external-model":%d}`, MaxTaskPreConsumeTokens)))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(`{"external-model":0}`))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(fmt.Sprintf(`{"external-model":%d}`, MaxTaskPreConsumeTokens+1)))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(`{"":100}`))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(`[]`))
}

func TestGetTaskPreConsumeTokensRejectsOutOfRangeStoredValue(t *testing.T) {
	original := taskBillingSetting.PreConsumeTokens
	taskBillingSetting.PreConsumeTokens = map[string]int{
		"valid":    520000,
		"too-high": MaxTaskPreConsumeTokens + 1,
	}
	t.Cleanup(func() {
		taskBillingSetting.PreConsumeTokens = original
	})

	tokens, ok := GetTaskPreConsumeTokens("valid")
	require.True(t, ok)
	require.Equal(t, 520000, tokens)

	_, ok = GetTaskPreConsumeTokens("too-high")
	require.False(t, ok)
}

func TestValidateBillingExpressionsRequiresTierMarker(t *testing.T) {
	// 新增模型（oldValue 无该 key）：强制 tier()
	require.NoError(t, ValidateBillingExpressionsJSON(`{"external-model":"tier(\"base\", c * 2)"}`, nil, nil))
	require.ErrorContains(t, ValidateBillingExpressionsJSON(`{"external-model":"c * 2"}`, nil, nil), "tier")
	require.Error(t, ValidateBillingExpressionsJSON(`{"external-model":"tier(\"base\", -1)"}`, nil, nil))
	// 未变更的存量表达式（oldValue 相同）：走 relaxed，允许无 tier()
	require.NoError(t, ValidateBillingExpressionsJSON(`{"legacy":"c * 2"}`, map[string]string{"legacy": "c * 2"}, nil))
	// 修改存量表达式：重新强制 tier()
	require.ErrorContains(t, ValidateBillingExpressionsJSON(`{"legacy":"c * 3"}`, map[string]string{"legacy": "c * 2"}, nil), "tier")
}

func TestValidateBillingExpressionsSupportsCommonTaskProbeParameters(t *testing.T) {
	expressions := `{
		"seedance-2.0-4k":"v1:tier(\"base\", param(\"_task.duration_seconds\") * 741114.000000)",
		"seedance-2-5-m":"v1:param(\"_task.has_video_input\") == true ? (param(\"_task.resolution\") == \"720p\" ? tier(\"720p_video\", param(\"_task.duration_seconds\") * 354838.709677) : tier(\"480p_video\", param(\"_task.duration_seconds\") * 164809.384164)) : (param(\"_task.resolution\") == \"720p\" ? tier(\"720p\", param(\"_task.duration_seconds\") * 145747.800587) : tier(\"480p\", param(\"_task.duration_seconds\") * 67741.935484))"
	}`

	taskModels := map[string]map[string]any{
		"seedance-2.0-4k": {},
		"seedance-2-5-m":  {},
	}
	require.NoError(t, ValidateBillingExpressionsJSON(expressions, nil, taskModels))
}

func TestValidateBillingExpressionsScopesProtocolSpecificTaskProbeParameters(t *testing.T) {
	expression := `{"custom-feicai-model":"v1:param(\"_task.billing_mode\") == \"per-second\" && param(\"_task.ratio\") == \"16:9\" ? tier(\"base\", param(\"_task.duration_seconds\") * param(\"_task.size_multiplier\")) : tier(\"invalid_probe\", -1)"}`
	feicaiFields := map[string]map[string]any{
		"custom-feicai-model": {
			"ratio":           "16:9",
			"size_multiplier": 1.0,
			"billing_mode":    "per-second",
		},
	}

	require.NoError(t, ValidateBillingExpressionsJSON(expression, nil, feicaiFields))
	require.Error(t, ValidateBillingExpressionsJSON(expression, nil, map[string]map[string]any{"custom-feicai-model": {}}))
}

func TestValidateBillingExpressionsRejectsUnknownNumericTaskParameter(t *testing.T) {
	expression := `{"external-model":"v1:tier(\"base\", param(\"_task.unsupported_numeric\") * 2)"}`

	require.ErrorContains(t, ValidateBillingExpressionsJSON(expression, nil, map[string]map[string]any{"external-model": {}}), "<nil> *")
}

func TestValidateBillingExpressionsDoesNotExposeTaskProbeToGenericModels(t *testing.T) {
	expression := `{"external-model":"v1:param(\"_task.duration_seconds\") == 5 ? tier(\"task\", c * 2) : tier(\"invalid_context\", -1)"}`

	require.Error(t, ValidateBillingExpressionsJSON(expression, nil, nil))
	require.NoError(t, ValidateBillingExpressionsJSON(expression, nil, map[string]map[string]any{"external-model": {}}))
}
