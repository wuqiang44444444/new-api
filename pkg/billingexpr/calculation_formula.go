package billingexpr

import (
	"fmt"
	"strings"
)

// Freeze arithmetic notation when the owning code records a step. This only
// formats already-computed operands; it never evaluates an expression or price.
// Persisting the notation prevents future UI/code changes from reinterpreting
// a historical host operation (normalization, discounts, budget bounds, etc.).
func recordedHostFormula(op string, a []string, result string) string {
	join := func(operator string) string { return strings.Join(a, " "+operator+" ") }
	switch op {
	case "sum", "+":
		return join("+")
	case "subtract", "-":
		return join("−")
	case "multiply", "group_ratio", "contract_ratio", "other_ratios", "per_call", "token_ratio":
		return join("×")
	case "quota_conversion":
		return fmt.Sprintf("%s ÷ %s × %s", a[0], a[2], a[1])
	case "batch_conversion", "ratio_estimate":
		return fmt.Sprintf("%s ÷ %s × %s × %s", a[0], a[1], a[2], a[3])
	case "token_components":
		return fmt.Sprintf("(%s + %s) × %s × %s", a[0], a[1], a[2], a[3])
	case "remove_other_ratios", "/":
		return join("÷")
	case "audio_input":
		return fmt.Sprintf("%s ÷ %s × %s × %s × %s", a[0], a[2], a[1], a[3], a[4])
	case "subtract_floor_zero":
		return "max(0, " + join("−") + ")"
	case "round", "truncate", "ceil":
		return op + "(" + strings.Join(a, ", ") + ")"
	case "refund":
		if len(a) > 0 {
			return a[0] + " − " + a[0]
		}
		return result
	case "free", "not_charged", "no_billable_usage":
		return "0"
	case "minimum_charge":
		return "max(1, " + strings.Join(a, ", ") + ")"
	case "default_if_zero":
		return "default(" + strings.Join(a, ", ") + ")"
	case "normalized_prompt":
		deductions := []string{a[0]}
		if a[5] != "true" && a[6] != "true" {
			deductions = append(deductions, a[1], a[2])
		}
		deductions = append(deductions, a[3])
		if a[7] == "true" {
			deductions = append(deductions, a[4])
		}
		return "max(0, " + strings.Join(deductions, " − ") + ")"
	case "cache_creation":
		if a[6] != "true" {
			return a[0] + " × " + a[3]
		}
		return fmt.Sprintf("max(0, %s − %s − %s) × %s + %s × %s + %s × %s", a[0], a[1], a[2], a[3], a[1], a[4], a[2], a[5])
	case "input_tokens", "output_tokens", "cache_tokens", "budget_constant", "budget", "usd_exchange_rate":
		return result
	case "estimated_tokens":
		return fmt.Sprintf("max(%s, %s) + %s", a[0], a[1], a[2])
	case "violation_fee":
		return "round(" + join("×") + ")"
	case "budget_range":
		return "[" + strings.Join(a, ", ") + "]"
	case "budget_+":
		return a[1] + " + " + a[3]
	case "budget_-":
		return a[1] + " − " + a[2]
	case "budget_*", "budget_/":
		operator := "×"
		if op == "budget_/" {
			operator = "÷"
		}
		return fmt.Sprintf("max(%s %s %s, %s %s %s, %s %s %s, %s %s %s)", a[0], operator, a[2], a[0], operator, a[3], a[1], operator, a[2], a[1], operator, a[3])
	case "budget_-unary":
		return "−" + a[0]
	case "budget_+unary":
		return a[1]
	default:
		return op + "(" + strings.Join(a, ", ") + ")"
	}
}
