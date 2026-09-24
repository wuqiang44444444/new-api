package billingexpr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRate(t *testing.T, rate float64) *ExchangeRateContext {
	t.Helper()
	ctx, err := NewExchangeRateContext(rate, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return ctx
}

func TestDisplayProjectionFoldsExchangeRate(t *testing.T) {
	expr := `tier("base", 0.30 / usd_exchange_rate() * 1000000)`

	at676, err := DisplayProjectionForWithRate(expr, testRate(t, 6.76))
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, at676.Status)
	require.Len(t, at676.Tiers, 1)
	assert.InDelta(t, 0.30/6.76, at676.Tiers[0].Constant, 1e-12)

	// 同一表达式在另一汇率下得到不同金额，缓存不串值。
	at7, err := DisplayProjectionForWithRate(expr, testRate(t, 7.0))
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, at7.Status)
	require.Len(t, at7.Tiers, 1)
	assert.InDelta(t, 0.30/7.0, at7.Tiers[0].Constant, 1e-12)
	assert.NotEqual(t, at676.Tiers[0].Constant, at7.Tiers[0].Constant)

	// 再次读取同汇率命中缓存且金额不变。
	again, err := DisplayProjectionForWithRate(expr, testRate(t, 6.76))
	require.NoError(t, err)
	assert.Equal(t, at676.Tiers[0].Constant, again.Tiers[0].Constant)

	// 无汇率上下文时明确不可展开，不猜价。
	unresolved, err := DisplayProjectionForWithRate(expr, nil)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusOpaque, unresolved.Status)
	assert.Equal(t, DisplayReasonExchangeRateUnresolved, unresolved.Reason)
	assert.Empty(t, unresolved.Tiers)
}

func TestTaskDisplayProjectionFoldsExchangeRate(t *testing.T) {
	expr := `tier("base", 1.0 / usd_exchange_rate())`

	proj, err := TaskDisplayProjectionForWithRate(expr, map[string]TaskUsageFieldInfo{}, testRate(t, 6.76))
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, proj.Status)
	require.Len(t, proj.Tiers, 1)
	assert.InDelta(t, 1.0/6.76, proj.Tiers[0].Constant, 1e-12)

	unresolved, err := TaskDisplayProjectionForWithRate(expr, map[string]TaskUsageFieldInfo{}, nil)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusOpaque, unresolved.Status)
	assert.Equal(t, DisplayReasonExchangeRateUnresolved, unresolved.Reason)
}

func TestDisplayProjectionRateFreeExpressionsUnaffected(t *testing.T) {
	expr := `tier("base", p * 2.5 + c * 15)`
	plain, err := DisplayProjectionFor(expr)
	require.NoError(t, err)
	withRate, err := DisplayProjectionForWithRate(expr, testRate(t, 6.76))
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, plain.Status)
	assert.Equal(t, DisplayStatusExact, withRate.Status)
	assert.Equal(t, plain.Tiers, withRate.Tiers)
}

func TestDisplayCachePreservesEachFrozenRateFact(t *testing.T) {
	for name, project := range map[string]func(*ExchangeRateContext) (*DisplayProjection, error){
		"tokens": func(rate *ExchangeRateContext) (*DisplayProjection, error) {
			return DisplayProjectionForWithRate(`tier("provenance", p * 7 / usd_exchange_rate())`, rate)
		},
		"task": func(rate *ExchangeRateContext) (*DisplayProjection, error) {
			return TaskDisplayProjectionForWithRate(`tier("provenance", u("seconds") * 7 / usd_exchange_rate())`, map[string]TaskUsageFieldInfo{"seconds": {Unit: "second"}}, rate)
		},
	} {
		t.Run(name, func(t *testing.T) {
			firstRate := testRate(t, 7)
			secondRate := *firstRate
			secondRate.FrozenAt = firstRate.FrozenAt.Add(24 * time.Hour)
			first, err := project(firstRate)
			require.NoError(t, err)
			second, err := project(&secondRate)
			require.NoError(t, err)
			require.NotNil(t, first.AppliedExchangeRate)
			require.NotNil(t, second.AppliedExchangeRate)
			assert.Equal(t, first.Tiers, second.Tiers)
			assert.Equal(t, *firstRate, *first.AppliedExchangeRate)
			assert.Equal(t, secondRate, *second.AppliedExchangeRate)
			first.AppliedExchangeRate.Rate = 1
			first.AppliedExchangeRate.FrozenAt = time.Time{}
			third, err := project(&secondRate)
			require.NoError(t, err)
			assert.Equal(t, secondRate, *third.AppliedExchangeRate)
			assert.Equal(t, secondRate, *second.AppliedExchangeRate)
			assert.Equal(t, 7.0, firstRate.Rate, "response metadata must not alias caller input")
		})
	}
}
