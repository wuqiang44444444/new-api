package billing_statement_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateContextThresholdsJSON(t *testing.T) {
	require.NoError(t, ValidateContextThresholdsJSON(`{"gpt-5":128000,"claude-sonnet":200000}`))

	tests := []string{
		`{"":128000}`,
		`{" gpt-5":128000}`,
		`{"gpt-5":0}`,
		`{"gpt-5":2147483648}`,
		`[]`,
		`null`,
	}
	for _, value := range tests {
		assert.Error(t, ValidateContextThresholdsJSON(value), value)
	}
}
