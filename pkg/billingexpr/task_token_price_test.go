package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskTokenPriceUsesFrozenBranchAndRate(t *testing.T) {
	const price = `u("billing_mode") == "per-token" ?
		tier("tokens", u("tokens") * (u("has_video_input") ? 2 : 4) / 1000000) :
		tier("seconds", u("duration_seconds") * 0.05)`
	facts := map[string]any{"billing_mode": "per-token", "has_video_input": true, "duration_seconds": 4}
	for _, tc := range []struct {
		tokens int
		want   float64
	}{{100000, 0.2}, {38830, 0.07766}, {0, 0}} {
		facts["tokens"] = tc.tokens
		input := RequestInput{Usage: facts}
		require.NoError(t, ValidateTaskTokenPrice(price, input))
		actual, trace, err := RunExprWithRequest(price, TokenParams{}, input)
		require.NoError(t, err)
		assert.Equal(t, "tokens", trace.MatchedTier)
		assert.InDelta(t, tc.want, actual, 1e-9)
	}
	facts["billing_mode"] = "per-second"
	require.Error(t, ValidateTaskTokenPrice(price, RequestInput{Usage: facts}))
}

func TestValidateTaskTokenPriceRejectsMeterDependentRatesAndFlatAmounts(t *testing.T) {
	input := RequestInput{Usage: map[string]any{"tokens": 100000, "duration_seconds": 4}}
	for _, price := range []string{
		`tier("seconds", u("tokens") >= 0 ? u("duration_seconds") : 0)`,
		`tier("with-flat-fee", u("tokens") / 1000000 + 0.1)`,
		`tier("switch-rate", u("tokens") * (u("tokens") > 10 ? 2 : 15))`,
		`tier("minimum", max(u("tokens") / 1000000, 0.2))`,
		`tier("time", u("tokens") * now().Hour())`,
	} {
		assert.Error(t, ValidateTaskTokenPrice(price, input), price)
	}
}
