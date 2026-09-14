package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskDisplayConditionalPriceFactorMatchesBilling(t *testing.T) {
	expression := `u("has_video_input") ? tier("reference_video", u("tokens") * (u("resolution") == "4k" ? 2.352 : u("resolution") == "1080p" ? 4.557 : 4.116) / 1000000) : tier("no_video", u("tokens") * (u("resolution") == "4k" ? 3.822 : u("resolution") == "1080p" ? 7.497 : 6.762) / 1000000)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 6)
	cases := []struct {
		video      bool
		resolution string
		price      float64
	}{
		{true, "4k", 2.352}, {true, "1080p", 4.557}, {true, "720p", 4.116},
		{false, "4k", 3.822}, {false, "1080p", 7.497}, {false, "720p", 6.762},
	}
	for i, tc := range cases {
		tier := projection.Tiers[i]
		assert.InDelta(t, tc.price, tier.UnitPrices["tokens"], 1e-12)
		assert.NotEmpty(t, tier.Condition)
		total, _, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{Usage: map[string]any{
			"tokens": float64(250000), "has_video_input": tc.video, "resolution": tc.resolution,
		}})
		require.NoError(t, err)
		assert.InDelta(t, total, tier.UnitPrices["tokens"]*0.25, 1e-12)
		for branchIndex, branch := range projection.Tiers {
			matched, _, err := RunExprWithRequest(branch.ConditionText+" ? 1 : 0", TokenParams{}, RequestInput{Usage: map[string]any{
				"has_video_input": tc.video, "resolution": tc.resolution,
			}})
			require.NoError(t, err)
			assert.Equal(t, branchIndex == i, matched == 1, "exactly the priced branch must match")
		}
	}
}

func TestTaskDisplayConditionalArithmeticPreservesUnitsAndFixedCharge(t *testing.T) {
	expression := `tier("base", 0.01 + (u("resolution") == "4k" ? 0.6 : 0.2) * u("duration_seconds") / (u("generate_audio") ? 1 : 2))`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 4)
	for i, price := range []float64{0.6, 0.3, 0.2, 0.1} {
		assert.InDelta(t, price, projection.Tiers[i].UnitPrices["duration_seconds"], 1e-12)
		assert.InDelta(t, 0.01, projection.Tiers[i].Constant, 1e-12)
	}
}

func TestTaskDisplayConditionalFactorFailsClosed(t *testing.T) {
	for _, expression := range []string{
		`tier("base", u("tokens") * (u("secret") ? 2 : 1))`,
		`tier("base", u("tokens") / (u("generate_audio") ? 0 : 2))`,
		`tier("base", u("tokens") * (u("generate_audio") ? u("tokens") : 2))`,
	} {
		t.Run(expression, func(t *testing.T) {
			projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
			require.NoError(t, err)
			assert.Equal(t, DisplayStatusOpaque, projection.Status)
			assert.Empty(t, projection.Tiers)
		})
	}
}
