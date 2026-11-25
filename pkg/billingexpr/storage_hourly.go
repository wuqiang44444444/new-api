package billingexpr

import (
	"errors"
	"math"
	"strings"
)

const StorageTokenHours = "storage_token_hours"

// StorageHourlyPrice validates the linear tariff used for both reservation
// and settlement. A single rate makes the bounded usage a safe upper bound.
func StorageHourlyPrice(expression string, exchangeRate *ExchangeRateContext) (float64, string, error) {
	invalid := errors.New(`expected a positive USD rate per million token-hours, for example tier("storage_hourly", u("storage_token_hours") * 0.0025 / 1000000)`)
	if len(expression) > 1024 || strings.TrimSpace(expression) == "" {
		return 0, "", invalid
	}
	keys := UsedUsageKeys(expression)
	if len(keys) != 1 || !keys[StorageTokenHours] || UsesChatQuantities(expression) || UsesFixedPricing(expression) {
		return 0, "", invalid
	}
	var frozenRate float64
	if UsesExchangeRate(expression) {
		if err := exchangeRate.Validate(); err != nil {
			return 0, "", err
		}
		frozenRate = exchangeRate.Rate
	}
	projection, err := TaskDisplayProjectionForWithRate(expression, map[string]TaskUsageFieldInfo{StorageTokenHours: {Unit: "token"}}, exchangeRate)
	if err != nil || projection.Status != DisplayStatusExact || len(projection.Tiers) != 1 ||
		len(projection.Rules) != 0 || projection.ConstantCharge != nil {
		return 0, "", invalid
	}
	tier := projection.Tiers[0]
	rate, ok := tier.UnitPrices[StorageTokenHours]
	if !ok || len(tier.UnitPrices) != 1 || tier.HasConstant || tier.Condition != nil ||
		tier.ConditionText != "" || len(tier.Conditions) != 0 || !(rate > 0) || math.IsInf(rate, 0) {
		return 0, "", invalid
	}
	usd, trace, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{
		Usage: map[string]any{StorageTokenHours: 1_000_000}, ExchangeRate: exchangeRate,
	})
	if err != nil || !(usd > 0) || math.IsInf(usd, 0) || math.Abs(usd-rate) > rate*1e-12 {
		return 0, "", invalid
	}
	exact, exactTier, err := storageHourlyExactRate(expression, frozenRate)
	if err != nil || exactTier != trace.MatchedTier {
		return 0, "", invalid
	}
	price, _ := exact.Float64()
	if !(price > 0) || math.IsInf(price, 0) || math.Abs(price-rate) > rate*1e-12 {
		return 0, "", invalid
	}
	return price, trace.MatchedTier, nil
}
