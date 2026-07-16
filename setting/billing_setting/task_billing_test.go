package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTaskPreConsumeTokensJSON(t *testing.T) {
	require.NoError(t, ValidateTaskPreConsumeTokensJSON(`{"external-model":250000}`))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(`{"external-model":0}`))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(`{"":100}`))
	require.Error(t, ValidateTaskPreConsumeTokensJSON(`[]`))
}

func TestValidateBillingExpressionsRequiresTierMarker(t *testing.T) {
	// 新增模型（oldValue 无该 key）：强制 tier()
	require.NoError(t, ValidateBillingExpressionsJSON(`{"external-model":"tier(\"base\", c * 2)"}`, nil))
	require.ErrorContains(t, ValidateBillingExpressionsJSON(`{"external-model":"c * 2"}`, nil), "tier")
	require.Error(t, ValidateBillingExpressionsJSON(`{"external-model":"tier(\"base\", -1)"}`, nil))
	// 未变更的存量表达式（oldValue 相同）：走 relaxed，允许无 tier()
	require.NoError(t, ValidateBillingExpressionsJSON(`{"legacy":"c * 2"}`, map[string]string{"legacy": "c * 2"}))
	// 修改存量表达式：重新强制 tier()
	require.ErrorContains(t, ValidateBillingExpressionsJSON(`{"legacy":"c * 3"}`, map[string]string{"legacy": "c * 2"}), "tier")
}
