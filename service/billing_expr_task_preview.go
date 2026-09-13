package service

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// Task previews have an explicit USD contract. This is synthetic input to the
// existing engine, not model validation, a funding session or a task snapshot.
func previewTaskUsageExpression(item BillingExprPreviewItem) BillingExprPreviewItemResult {
	result := BillingExprPreviewItemResult{Key: item.Key}
	if err := billingexpr.UnknownIdentifier(item.Expression); err != nil {
		result.Error = err.Error()
		return result
	}
	variables := billingexpr.UsedVars(item.Expression)
	for _, name := range []string{"p", "c", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao", "len"} {
		if variables[name] {
			result.Error = "Token variables cannot be evaluated as task USD pricing."
			return result
		}
	}
	if item.Sample == nil {
		result.Error = "Task usage samples are required."
		return result
	}
	if len(item.Sample.Usage) > 64 {
		result.Error = "Too many task usage sample fields."
		return result
	}
	for key := range billingexpr.UsedUsageKeys(item.Expression) {
		if _, exists := item.Sample.Usage[key]; !exists {
			result.Error = "Provide a sample for every referenced usage field."
			return result
		}
	}
	for _, value := range item.Sample.Usage {
		switch value := value.(type) {
		case float64:
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > math.MaxInt32 {
				result.Error = "Task usage samples must be finite and between 0 and 2147483647."
				return result
			}
		case string, bool:
		default:
			result.Error = "Task usage samples must contain numbers, strings or booleans."
			return result
		}
	}
	body, err := common.Marshal(item.Sample.Body)
	if err != nil {
		result.Error = "invalid trial request context"
		return result
	}
	pricingTime := time.Now()
	if item.Sample.PricingTime != "" {
		pricingTime, err = time.Parse(time.RFC3339, item.Sample.PricingTime)
		if err != nil {
			result.Error = "pricing_time must be an RFC3339 timestamp"
			return result
		}
	}
	snapshot := &billingexpr.BillingSnapshot{
		ExprString: item.Expression, ExprHash: billingexpr.ExprHashString(item.Expression),
		ExprVersion: billingexpr.ExprVersion(item.Expression), TaskUsageBilling: true,
		QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1,
	}
	outcome, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, billingexpr.RequestInput{
		Usage: item.Sample.Usage, Body: body, Headers: item.Sample.Headers, PricingTime: &pricingTime,
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	amount := outcome.ActualQuotaBeforeGroup / common.QuotaPerUnit
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		result.Error = "expression must produce a finite non-negative cost"
		return result
	}
	result.Evaluation = &BillingExprPreviewEvaluation{
		PricingTime: pricingTime.Format(time.RFC3339), UsageSemantic: "task",
		RawCostUSD: amount, Quota: outcome.ActualQuotaAfterGroup, MatchedTier: outcome.MatchedTier,
		RequestRules: outcome.RequestRules, Saturated: outcome.Clamp != nil,
	}
	return result
}
