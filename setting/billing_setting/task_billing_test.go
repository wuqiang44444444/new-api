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

func TestGenericExpressionValidationContract(t *testing.T) {
	for _, tc := range []struct {
		expression, old string
		valid           bool
	}{
		{`tier("base", c * 2)`, "", true}, {`c * 2`, "", false},
		{`tier("base", -1)`, "", false}, {`c * 2`, `c * 2`, true}, {`c * 3`, `c * 2`, false},
		{`param("_task.duration_seconds") == 5 ? tier("task", c * 2) : tier("invalid", -1)`, "", false},
	} {
		t.Run(tc.expression, func(t *testing.T) {
			err := ValidateOneBillingExpression("generic", tc.expression, tc.old)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
