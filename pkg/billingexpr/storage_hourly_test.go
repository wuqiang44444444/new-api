package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageHourlyPriceUsesOnlyPositiveLinearStoredUsage(t *testing.T) {
	expression := `tier("standard", u("storage_token_hours") * 0.0025 / 1000000)`
	price, tier, err := StorageHourlyPrice(expression, nil)
	require.NoError(t, err)
	assert.Equal(t, "standard", tier)
	assert.InDelta(t, 0.0025, price, 1e-12)
	usd, _, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{Usage: map[string]any{StorageTokenHours: int64(80_000)}})
	require.NoError(t, err)
	assert.InDelta(t, 0.0002, usd, 1e-12)

	for _, bad := range []string{
		`u("storage_token_hours") * (-1) / 1000000`,
		`u("storage_token_hours") * u("storage_token_hours")`,
		`u("storage_token_hours") * 0.0025 / 1000000 + 1`,
		`u("storage_token_hours") * 1 + p`,
		`u("some_other_usage") * 1`,
		`u("storage_token_hours") > 100 ? tier("a", u("storage_token_hours") * 1) : tier("b", u("storage_token_hours") * 2)`,
	} {
		_, _, err := StorageHourlyPrice(bad, nil)
		assert.Error(t, err, bad)
	}
}
