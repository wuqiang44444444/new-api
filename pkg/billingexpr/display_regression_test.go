package billingexpr

import (
	"github.com/QuantumNous/new-api/common"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayDivisionPreservesEngineSemantics(t *testing.T) {
	for _, expression := range []string{
		`tier("base", p * 2) / (hour("Asia/Shanghai") >= 9 ? 2 : 1)`,
		`tier("base", p * 2) / (hour("Asia/Shanghai") >= 9 ? 2 : 4)`,
	} {
		projection, err := BuildDisplayProjection(expression)
		require.NoError(t, err)
		require.Equal(t, DisplayStatusExact, projection.Status)
		for _, hour := range []int{8, 10} {
			at := time.Date(2026, 9, 7, hour, 0, 0, 0, time.FixedZone("Shanghai", 8*3600))
			cost, _, err := RunExprByHashWithRequest(expression, ExprHashString(expression), TokenParams{P: 1_000_000}, RequestInput{PricingTime: &at})
			require.NoError(t, err)
			factor := projection.Rules[0].Fallback
			if hour >= 9 {
				factor = projection.Rules[0].Multiplier
			}
			assert.InDelta(t, cost/1_000_000, projection.Tiers[0].UnitPrices["p"]*factor, 1e-12)
		}
	}
}

func TestDisplayRejectsNonlinearPriceDenominator(t *testing.T) {
	projection, err := BuildDisplayProjection(`1 / tier("base", p * 2)`)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusOpaque, projection.Status)
	assert.Empty(t, projection.Tiers)
}

func TestDisplayFixedChargeUsesRealDollars(t *testing.T) {
	projection, err := BuildDisplayProjection(`tier("base", p * 2 + 10000) + 20000`)
	require.NoError(t, err)
	require.Equal(t, DisplayStatusExact, projection.Status)
	assert.InDelta(t, 0.01, projection.Tiers[0].Constant, 1e-12)
	require.NotNil(t, projection.ConstantCharge)
	assert.InDelta(t, 0.02, *projection.ConstantCharge, 1e-12)
}

func TestDisplayZeroFallbackSurvivesJSON(t *testing.T) {
	projection, err := BuildDisplayProjection(`tier("base", p * 2) * (hour("Asia/Shanghai") >= 9 ? 2 : 0)`)
	require.NoError(t, err)
	require.Equal(t, DisplayStatusExact, projection.Status)
	require.Len(t, projection.Scenarios, 2)
	assert.Zero(t, projection.Scenarios[0].Tiers[0].UnitPrices["p"])
	encoded, err := common.Marshal(projection)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"fallback":0`)
}

func TestDisplayConditionalStandaloneChargeIsNotMisrepresented(t *testing.T) {
	projection, err := BuildDisplayProjection(`hour("Asia/Shanghai") >= 9 ? 20000 : 10000`)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusOpaque, projection.Status)
	assert.Empty(t, projection.Tiers)
	assert.Nil(t, projection.ConstantCharge)
}

func TestDisplayFrontendFixturesMatchBackendContract(t *testing.T) {
	data, err := os.ReadFile("../../web/src/features/pricing/__tests__/billing-display-fixtures.json")
	require.NoError(t, err)
	var fixtures map[string]DisplayProjection
	require.NoError(t, common.Unmarshal(data, &fixtures))
	require.NotEmpty(t, fixtures)
	for expression, fixture := range fixtures {
		t.Run(expression, func(t *testing.T) {
			actual, err := BuildDisplayProjection(expression)
			require.NoError(t, err)
			assert.Equal(t, fixture, *actual)
		})
	}
}
