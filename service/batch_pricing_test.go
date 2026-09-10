package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func frozenSnapshot(expr string, contract *hosttypes.ContractBillingFact) *model.BatchFrozenSnapshot {
	return &model.BatchFrozenSnapshot{
		Expr: expr, ExprHash: billingexpr.ExprHashString(expr),
		PricingTime:  time.Date(2026, 9, 9, 15, 30, 0, 0, time.UTC).Unix(), // 23:30 Shanghai
		QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1,
		ContractFact: contract,
	}
}

func TestBatchLineQuotaUsesFrozenPricingTimeAcrossWallClock(t *testing.T) {
	nightExpr := `hour("Asia/Shanghai") >= 22 || hour("Asia/Shanghai") < 6 ? tier("night", p * 2 + c * 4) : tier("day", p * 3 + c * 6)`
	usage := BatchLineUsage{InputTokens: 1000, OutputTokens: 100}

	frozen := frozenSnapshot(nightExpr, nil)
	quotaAt2330, clamp, err := computeBatchLineModelQuota(frozen, usage)
	require.NoError(t, err)
	require.Nil(t, clamp)
	assert.Greater(t, quotaAt2330, 0)

	// A different frozen instant must produce the daytime price even if the
	// wall clock matches the night window at evaluation time.
	dayFrozen := frozenSnapshot(nightExpr, nil)
	dayFrozen.PricingTime = time.Date(2026, 9, 9, 4, 0, 0, 0, time.UTC).Unix() // 12:00 Shanghai
	quotaAtNoon, _, err := computeBatchLineModelQuota(dayFrozen, usage)
	require.NoError(t, err)
	assert.Greater(t, quotaAtNoon, quotaAt2330)

	// Deterministic: same frozen instant, same amount.
	again, _, err := computeBatchLineModelQuota(frozen, usage)
	require.NoError(t, err)
	assert.Equal(t, quotaAt2330, again)
}

func TestBatchLineQuotaNormalizesCachedTokensOnlyWhenPriced(t *testing.T) {
	cacheExpr := `tier("base", p * 3 + c * 6 + cr * 0.3)`
	plainExpr := `tier("base", p * 3 + c * 6)`
	usage := BatchLineUsage{InputTokens: 1000, OutputTokens: 500, CachedTokens: 800}

	withCache, _, err := computeBatchLineModelQuota(frozenSnapshot(cacheExpr, nil), usage)
	require.NoError(t, err)
	plain, _, err := computeBatchLineModelQuota(frozenSnapshot(plainExpr, nil), usage)
	require.NoError(t, err)
	assert.Greater(t, plain, withCache, "cache-priced expressions exclude cached tokens from p")
}

func TestBatchLineQuotaSaturatesInsteadOfOverflowing(t *testing.T) {
	expr := `tier("base", p * 1000000 + c * 1000000)`
	absurd := BatchLineUsage{InputTokens: int64(1) << 40, OutputTokens: int64(1) << 40}
	quota, clamp, err := computeBatchLineModelQuota(frozenSnapshot(expr, nil), absurd)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, quota, 0, "no negative charge from overflow")
	assert.NotNil(t, clamp, "saturation must be reported for audit")
}

func TestBatchLineQuotaAppliesContractDiscountOnce(t *testing.T) {
	expr := `tier("base", p * 3 + c * 6)`
	fact := &hosttypes.ContractBillingFact{RatioUnits: 50_000_000} // 0.5
	frozen := frozenSnapshot(expr, fact)
	usage := BatchLineUsage{InputTokens: 1000, OutputTokens: 500}

	full, _, err := computeBatchLineModelQuota(frozenSnapshot(expr, nil), usage)
	require.NoError(t, err)
	discounted, _, err := computeBatchLineModelQuota(frozen, usage)
	require.NoError(t, err)

	afterContract, err := ApplyBatchContractRatio(discounted, fact)
	require.NoError(t, err)
	assert.True(t, afterContract.Equal(decimal.NewFromInt(int64(full)).Mul(decimal.NewFromInt(5)).Div(decimal.NewFromInt(10))),
		"contract discount multiplies the line model cost exactly once")
}

func TestEstimateBatchJobQuotaSumsLineBudgetsPessimistically(t *testing.T) {
	expr := `tier("base", p * 3 + c * 6)`
	frozen := frozenSnapshot(expr, nil)
	lines := []BatchLineEstimate{
		{CustomId: "a", InputEst: 1000, OutputCap: 500},
		{CustomId: "b", InputEst: 2000, OutputCap: 100},
	}
	estimate, err := estimateBatchJobQuota(frozen, lines)
	require.NoError(t, err)
	assert.Greater(t, estimate, 0)

	// Zero-output-cap lines still hold a budget from the input estimate.
	zeroCap, err := estimateBatchJobQuota(frozen, []BatchLineEstimate{{CustomId: "c", InputEst: 1000}})
	require.NoError(t, err)
	assert.Greater(t, zeroCap, 0)
}

func TestBatchBudgetCoversNonMonotonicTierAndQuantity(t *testing.T) {
	frozen := frozenSnapshot(`len < 100 ? tier("short", p * 100) : tier("long", p)`, nil)
	budget, err := estimateBatchJobQuota(frozen, []BatchLineEstimate{{CustomId: "a", InputEst: 1000, OutputCap: 10}})
	require.NoError(t, err)
	actual, _, err := computeBatchLineFinalQuota(frozen, BatchLineUsage{InputTokens: 99})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, budget, actual)
}

func TestBatchPricingFreezesOnlyReferencedRequestParameters(t *testing.T) {
	expression := `(param("service_tier") == "priority" ? 2 : 1) * (p * 2 + c * 4)`
	params, err := freezeBatchPricingParameters(expression, []byte(`{"custom_id":"a","body":{"service_tier":"priority","messages":[{"content":"private prompt"}]}}`))
	require.NoError(t, err)
	assert.NotContains(t, string(params["a"]), "private prompt")
	frozen := frozenSnapshot(expression, nil)
	frozen.LineParams = params
	charge, _, err := computeBatchLineFinalQuota(frozen, BatchLineUsage{CustomId: "a", InputTokens: 10, OutputTokens: 5})
	require.NoError(t, err)
	assert.Equal(t, 40, charge)
	_, err = freezeBatchPricingParameters(`param("messages.0.content") == "x" ? p : c`, nil)
	require.Error(t, err)
}

func TestBatchFinalQuotaRoundsAfterContractRatio(t *testing.T) {
	frozen := frozenSnapshot(`p * 3.8`, &hosttypes.ContractBillingFact{RatioUnits: 60000000})
	quota, _, err := computeBatchLineFinalQuota(frozen, BatchLineUsage{InputTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, quota, "1.9 quota * 0.6 rounds once to 1")
}
