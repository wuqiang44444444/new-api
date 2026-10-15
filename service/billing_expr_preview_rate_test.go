package service

import (
	"fmt"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizedPreviewRejectsNonFiniteTokens(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		result := PreviewBillingExpressions([]BillingExprPreviewItem{{
			Expression: `tier("base", p * 2)`,
			Sample:     &BillingExprPreviewSample{UsageSemantic: "normalized", NormalizedUsage: map[string]*float64{"p": &value}},
		}})
		require.Len(t, result, 1)
		assert.NotEmpty(t, result[0].Error)
		assert.Nil(t, result[0].Evaluation)
	}
}

func TestTaskPreviewReturnsAuthoritativeExchangeRate(t *testing.T) {
	prior := operation_setting.USDExchangeRate
	t.Cleanup(func() { require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(prior))) })
	for _, rate := range []float64{7, 10} {
		require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(rate)))
		result := PreviewBillingExpressions([]BillingExprPreviewItem{{
			Expression: `tier("task", u("seconds") * 7 / usd_exchange_rate())`, TaskUsage: true,
			Sample: &BillingExprPreviewSample{Usage: map[string]any{"seconds": 2.0}},
		}})
		require.Len(t, result, 1)
		require.Empty(t, result[0].Error)
		require.NotNil(t, result[0].Evaluation)
		assert.InDelta(t, 14/rate, result[0].Evaluation.RawCostUSD, 1e-12)
		require.NotNil(t, result[0].Evaluation.UsdExchangeRate)
		assert.Equal(t, rate, result[0].Evaluation.UsdExchangeRate.Rate)
	}
	require.Error(t, operation_setting.SetUSDExchangeRate("0"))
	result := PreviewBillingExpressions([]BillingExprPreviewItem{{
		Expression: `tier("task", u("seconds") * 7 / usd_exchange_rate())`, TaskUsage: true,
		Sample: &BillingExprPreviewSample{Usage: map[string]any{"seconds": 2.0}},
	}})
	require.Len(t, result, 1)
	assert.NotEmpty(t, result[0].Error)
	assert.Nil(t, result[0].Evaluation)
}
