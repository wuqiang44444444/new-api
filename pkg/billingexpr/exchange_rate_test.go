package billingexpr

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frozenRateCtx builds a valid frozen context for tests.
func frozenRateCtx(t *testing.T, rate float64) *ExchangeRateContext {
	t.Helper()
	ctx, err := NewExchangeRateContext(rate, time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return ctx
}

const seedreamExpr = `tier("1K", (0.30 + (param("input_image_count") == nil ? 0 : max(param("input_image_count") - 1, 0) * 0.02)) / usd_exchange_rate() * 1000000)`

func TestExchangeRateContextValidate(t *testing.T) {
	for _, rate := range []float64{6.76, 7, 0.0001, 1e9} {
		assert.NoError(t, ValidateExchangeRateValue(rate), "rate %v must be accepted", rate)
	}
	for _, rate := range []float64{0, -1, -0.5, math.NaN(), math.Inf(1), math.Inf(-1)} {
		assert.Error(t, ValidateExchangeRateValue(rate), "rate %v must be rejected", rate)
	}
	var missing *ExchangeRateContext
	assert.Error(t, missing.Validate())
}

func TestUsdExchangeRateFunctionSemantics(t *testing.T) {
	expr := `tier("base", 2.0 / usd_exchange_rate())`
	body := []byte(`{}`)

	out, _, err := RunExprWithRequest(expr, TokenParams{}, RequestInput{Body: body, ExchangeRate: frozenRateCtx(t, 6.76)})
	require.NoError(t, err)
	assert.Equal(t, 2.0/6.76, out)

	// Missing, zero and negative contexts fail closed; no fallback value.
	for _, bad := range []*ExchangeRateContext{nil, {SourceKey: UsdExchangeRateSourceKey, Rate: 0}, {SourceKey: UsdExchangeRateSourceKey, Rate: -7}} {
		_, _, err := RunExprWithRequest(expr, TokenParams{}, RequestInput{Body: body, ExchangeRate: bad})
		require.Error(t, err)
	}
}

func TestUsesExchangeRateDetection(t *testing.T) {
	assert.True(t, UsesExchangeRate(seedreamExpr))
	// Untaken branches still count: the whole expression is detected.
	assert.True(t, UsesExchangeRate(`1 == 2 ? tier("a", 1.0) : tier("b", usd_exchange_rate())`))
	assert.False(t, UsesExchangeRate(`tier("base", p * 2.5 + c * 15)`))
	assert.False(t, UsesExchangeRate(""))
	assert.False(t, UsesExchangeRate(`tier("broken",`))
}

func TestSettlementUsesFrozenRateOverExternalValue(t *testing.T) {
	expr := `tier("base", 2.0 / usd_exchange_rate())`
	snap := &BillingSnapshot{
		BillingMode:     "tiered_expr",
		ExprString:      expr,
		ExprHash:        ExprHashString(expr),
		QuotaPerUnit:    500000,
		GroupRatio:      1,
		UsdExchangeRate: frozenRateCtx(t, 6.76),
	}
	// An externally supplied different rate must be overridden by the snapshot.
	result, err := ComputeTieredQuotaWithRequest(snap, TokenParams{}, RequestInput{ExchangeRate: frozenRateCtx(t, 7.0)})
	require.NoError(t, err)
	assert.InDelta(t, 2.0/6.76/1_000_000*500000, result.ActualQuotaBeforeGroup, 1e-9)

	// A rate-dependent expression without a frozen snapshot fact cannot settle.
	snap.UsdExchangeRate = nil
	_, err = ComputeTieredQuotaWithRequest(snap, TokenParams{}, RequestInput{})
	require.Error(t, err)
}

func TestSnapshotExchangeRateRoundTrip(t *testing.T) {
	expr := `tier("base", usd_exchange_rate() * 0.5)`
	frozen := frozenRateCtx(t, 6.76)
	snap := &BillingSnapshot{ExprString: expr, ExprHash: ExprHashString(expr), UsdExchangeRate: frozen}
	encoded, err := common.Marshal(snap)
	require.NoError(t, err)
	var restored BillingSnapshot
	require.NoError(t, common.Unmarshal(encoded, &restored))
	require.NotNil(t, restored.UsdExchangeRate)
	assert.Equal(t, 6.76, restored.UsdExchangeRate.Rate)
	assert.Equal(t, UsdExchangeRateSourceKey, restored.UsdExchangeRate.SourceKey)
	assert.True(t, restored.UsdExchangeRate.FrozenAt.Equal(frozen.FrozenAt))
}

func TestSeedreamPerImageContract(t *testing.T) {
	cases := []struct {
		inputImages int
		expected    float64
	}{
		{0, 0.30 / 6.76},
		{1, 0.30 / 6.76},
		{2, 0.32 / 6.76},
		{10, 0.48 / 6.76},
	}
	for _, tc := range cases {
		body := []byte(`{"input_image_count":` + strconv.Itoa(tc.inputImages) + `}`)
		out, _, err := RunExprWithRequest(seedreamExpr, TokenParams{}, RequestInput{Body: body, ExchangeRate: frozenRateCtx(t, 6.76)})
		require.NoError(t, err, "input images %d", tc.inputImages)
		assert.InDelta(t, tc.expected, out/1_000_000, 1e-15, "input images %d", tc.inputImages)
	}
}

func TestRequestContextCannotInjectExchangeRate(t *testing.T) {
	// Body keys that mirror the function name must have no effect.
	body := []byte(`{"usd_exchange_rate": 1, "rate": 1}`)
	out, _, err := RunExprWithRequest(`tier("base", 2.0 / usd_exchange_rate())`, TokenParams{}, RequestInput{Body: body, ExchangeRate: frozenRateCtx(t, 6.76)})
	require.NoError(t, err)
	assert.Equal(t, 2.0/6.76, out)
}

func TestExchangeRateConversionUnitsDifferByMode(t *testing.T) {
	// 普通表达式系数为 $/1M tokens：quota = cost / 1M * QuotaPerUnit * group。
	// 任务表达式直接输出 USD：quota = cost * QuotaPerUnit * group。两条路径
	// 都只换汇一次，不相差百万倍。
	tokenExpr := seedreamExpr
	tokenSnap := &BillingSnapshot{
		ExprString: tokenExpr, ExprHash: ExprHashString(tokenExpr),
		QuotaPerUnit: 500000, GroupRatio: 1, UsdExchangeRate: frozenRateCtx(t, 6.76),
	}
	tokenResult, err := ComputeTieredQuotaWithRequest(tokenSnap, TokenParams{}, RequestInput{Body: []byte(`{}`)})
	require.NoError(t, err)
	// 表达式自带 *1000000，quota 换算除回：净效果只有一次换汇。
	assert.InDelta(t, 0.30/6.76*500000, tokenResult.ActualQuotaBeforeGroup, 1e-9)

	taskExpr := `tier("base", 2.0 / usd_exchange_rate())`
	taskSnap := &BillingSnapshot{
		ExprString: taskExpr, ExprHash: ExprHashString(taskExpr),
		QuotaPerUnit: 500000, GroupRatio: 1, TaskUsageBilling: true, UsdExchangeRate: frozenRateCtx(t, 6.76),
	}
	taskResult, err := ComputeTieredQuotaWithRequest(taskSnap, TokenParams{}, RequestInput{})
	require.NoError(t, err)
	assert.InDelta(t, 2.0/6.76*500000, taskResult.ActualQuotaBeforeGroup, 1e-9)
}

func TestExchangeRateReferencesMustUseDirectCalls(t *testing.T) {
	for _, expression := range []string{
		`let fx = usd_exchange_rate; tier("base", p > 2000000 ? p * 7 / fx() : p)`,
		`true ? tier("base", 1) : tier("other", (let fx = usd_exchange_rate; fx()))`,
		`tier("base", $env.usd_exchange_rate())`,
		`tier("base", $env["usd_" + "exchange_rate"]())`,
		`let env = $env; tier("base", env.usd_exchange_rate())`,
		`tier("base", usd_exchange_rate(7))`,
	} {
		_, err := CompileFromCache(expression)
		require.Error(t, err, expression)
	}
	// Rate values may still be assigned after a direct call, and ordinary
	// literal environment access must retain its existing meaning.
	expression := `let fx = usd_exchange_rate(); tier("base", $env.p * 7 / fx)`
	assert.True(t, UsesExchangeRate(expression))
	value, _, err := RunExprWithRequest(expression, TokenParams{P: 1000}, RequestInput{ExchangeRate: frozenRateCtx(t, 7)})
	require.NoError(t, err)
	assert.Equal(t, float64(1000), value)
}

func TestExchangeRateRequiredEvenForUntakenBranches(t *testing.T) {
	for _, expression := range []string{
		`p > 1 ? tier("a", 7 / usd_exchange_rate()) : tier("b", 1)`,
		`true ? tier("a", 1) : tier("b", 7 / usd_exchange_rate())`,
	} {
		require.True(t, UsesExchangeRate(expression))
		for _, rate := range []*ExchangeRateContext{nil, {Rate: 0}, {Rate: -7}, {Rate: math.NaN()}, {Rate: math.Inf(1)}} {
			_, _, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{ExchangeRate: rate})
			require.Error(t, err)
			_, _, err = RunExprByHashWithRequest(expression, ExprHashString(expression), TokenParams{}, RequestInput{ExchangeRate: rate})
			require.Error(t, err)
		}
		snapshot := &BillingSnapshot{ExprString: expression, ExprHash: ExprHashString(expression), QuotaPerUnit: 500000, GroupRatio: 1}
		_, err := ComputeTieredQuotaWithRequest(snapshot, TokenParams{}, RequestInput{ExchangeRate: frozenRateCtx(t, 7)})
		require.Error(t, err, "a caller cannot replace a missing snapshot fact")
		snapshot.UsdExchangeRate = frozenRateCtx(t, 7)
		result, err := ComputeTieredQuota(snapshot, TokenParams{})
		require.NoError(t, err)
		assert.Equal(t, 0.5, result.ActualQuotaBeforeGroup)
	}
}
