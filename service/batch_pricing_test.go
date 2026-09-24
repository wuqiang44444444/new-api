package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	hosttypes "github.com/QuantumNous/new-api/types"
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
	_, quotaAt2330, clamp, _, err := computeBatchLineCalculation(frozen, usage)
	require.NoError(t, err)
	require.Nil(t, clamp)
	assert.Greater(t, quotaAt2330, 0)

	// A different frozen instant must produce the daytime price even if the
	// wall clock matches the night window at evaluation time.
	dayFrozen := frozenSnapshot(nightExpr, nil)
	dayFrozen.PricingTime = time.Date(2026, 9, 9, 4, 0, 0, 0, time.UTC).Unix() // 12:00 Shanghai
	_, quotaAtNoon, _, _, err := computeBatchLineCalculation(dayFrozen, usage)
	require.NoError(t, err)
	assert.Greater(t, quotaAtNoon, quotaAt2330)

	// Deterministic: same frozen instant, same amount.
	_, again, _, _, err := computeBatchLineCalculation(frozen, usage)
	require.NoError(t, err)
	assert.Equal(t, quotaAt2330, again)
}

func TestBatchLineQuotaNormalizesCachedTokensOnlyWhenPriced(t *testing.T) {
	cacheExpr := `tier("base", p * 3 + c * 6 + cr * 0.3)`
	plainExpr := `tier("base", p * 3 + c * 6)`
	usage := BatchLineUsage{InputTokens: 1000, OutputTokens: 500, CachedTokens: 800}

	_, withCache, _, _, err := computeBatchLineCalculation(frozenSnapshot(cacheExpr, nil), usage)
	require.NoError(t, err)
	_, plain, _, _, err := computeBatchLineCalculation(frozenSnapshot(plainExpr, nil), usage)
	require.NoError(t, err)
	assert.Greater(t, plain, withCache, "cache-priced expressions exclude cached tokens from p")
}

func TestBatchLineQuotaSaturatesInsteadOfOverflowing(t *testing.T) {
	expr := `tier("base", p * 1000000 + c * 1000000)`
	absurd := BatchLineUsage{InputTokens: int64(1) << 40, OutputTokens: int64(1) << 40}
	_, quota, clamp, _, err := computeBatchLineCalculation(frozenSnapshot(expr, nil), absurd)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, quota, 0, "no negative charge from overflow")
	assert.NotNil(t, clamp, "saturation must be reported for audit")
}

func TestBatchLineQuotaAppliesContractDiscountOnce(t *testing.T) {
	expr := `tier("base", p * 3 + c * 6)`
	fact := &hosttypes.ContractBillingFact{RatioUnits: 50_000_000} // 0.5
	frozen := frozenSnapshot(expr, fact)
	usage := BatchLineUsage{InputTokens: 1000, OutputTokens: 500}

	_, full, _, _, err := computeBatchLineCalculation(frozenSnapshot(expr, nil), usage)
	require.NoError(t, err)
	modelQuota, discounted, _, calculation, err := computeBatchLineCalculation(frozen, usage)
	require.NoError(t, err)

	assert.Equal(t, full, modelQuota)
	assert.Equal(t, full/2, discounted, "contract discount applies once to the actual production result")
	assert.Equal(t, discounted, calculation.Quota)
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
	_, actual, _, _, err := computeBatchLineCalculation(frozen, BatchLineUsage{InputTokens: 99})
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
	_, charge, _, _, err := computeBatchLineCalculation(frozen, BatchLineUsage{CustomId: "a", InputTokens: 10, OutputTokens: 5})
	require.NoError(t, err)
	assert.Equal(t, 40, charge)
	_, err = freezeBatchPricingParameters(`param("messages.0.content") == "x" ? p : c`, nil)
	require.Error(t, err)
}

func TestBatchFinalQuotaRoundsAfterContractRatio(t *testing.T) {
	frozen := frozenSnapshot(`p * 3.8`, &hosttypes.ContractBillingFact{RatioUnits: 60000000})
	_, quota, _, _, err := computeBatchLineCalculation(frozen, BatchLineUsage{InputTokens: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, quota, "1.9 quota * 0.6 rounds once to 1")
}
