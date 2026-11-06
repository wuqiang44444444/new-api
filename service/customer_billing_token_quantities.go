package service

import (
	"math"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// frozenCustomerTokenQuantities explains recorded normalization, never repeats
// settlement or infers missing media quantities from aggregate token counts.
func frozenCustomerTokenQuantities(log *model.Log, other map[string]any, expression string, labels map[string]string) map[string]float64 {
	facts, ok := other["billing_tokens"].(map[string]any)
	if !ok || other["usage_semantic"] == "anthropic" || other["claude"] == true {
		return nil
	}
	used := billingexpr.UsedVars(expression)
	quantities := make(map[string]float64)
	input, output := float64(0), float64(0)
	for _, key := range []string{"p", "c", "cr", "cc", "cc1h", "img", "img_cr", "ai", "img_o", "ao"} {
		if key != "p" && key != "c" && !used[key] {
			continue
		}
		value, valid := customerBillingNumber(facts, key)
		if !valid || value > math.MaxInt32 || math.Trunc(value) != value {
			return nil
		}
		quantities[key] = value
		if key == "c" || key == "img_o" || key == "ao" {
			output += value
		} else {
			input += value
		}
	}
	// Normalized subcategories must reconstruct the recorded OpenAI totals.
	// Overlapping/legacy normalization cannot establish a complete explanation.
	if input != float64(log.PromptTokens) || output != float64(log.CompletionTokens) {
		return nil
	}
	path, _ := other["request_path"].(string)
	if path == "/v1/images/generations" || path == "/v1/images/edits" {
		if used["img"] {
			labels["p"] = "Text Input"
		}
		imageOutput, imageKnown := customerBillingNumber(facts, "img_o")
		if !used["img_o"] && imageKnown && imageOutput == output && output > 0 {
			labels["c"] = "Image Out"
		}
	}
	return quantities
}
