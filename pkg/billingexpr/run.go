package billingexpr

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/tidwall/gjson"
)

// RunExpr compiles (with cache) and executes an expression string.
// The environment exposes:
//   - p, c             — prompt / completion tokens (auto-excluding separately-priced sub-categories)
//   - len              — total input context length for tier conditions (never reduced by sub-category exclusion)
//   - cr, cc, cc1h     — cache read / creation / creation-1h tokens
//   - tier(name, value) — trace callback that records which tier matched
//   - max, min, abs, ceil, floor — standard math helpers
//
// Returns the resulting float64 quota (before group ratio) and a TraceResult
// with side-channel info captured by tier() during execution.
func RunExpr(exprStr string, params TokenParams) (float64, TraceResult, error) {
	return RunExprWithRequest(exprStr, params, RequestInput{})
}

func RunExprWithRequest(exprStr string, params TokenParams, request RequestInput) (float64, TraceResult, error) {
	entry, err := compileEntryFromCacheByHash(exprStr, ExprHashString(exprStr))
	if err != nil {
		if request.RecordCalculation {
			return 0, TraceResult{}, calculationError{cause: err, phase: "compile"}
		}
		return 0, TraceResult{}, err
	}
	return runProgram(entry, params, request)
}

// RunExprByHash is like RunExpr but accepts a pre-computed hash for the cache
// lookup, avoiding a redundant SHA-256 computation when the caller already
// holds BillingSnapshot.ExprHash.
func RunExprByHash(exprStr, hash string, params TokenParams) (float64, TraceResult, error) {
	return RunExprByHashWithRequest(exprStr, hash, params, RequestInput{})
}

func RunExprByHashWithRequest(exprStr, hash string, params TokenParams, request RequestInput) (float64, TraceResult, error) {
	entry, err := compileEntryFromCacheByHash(exprStr, hash)
	if err != nil {
		if request.RecordCalculation {
			return 0, TraceResult{}, calculationError{cause: err, phase: "compile"}
		}
		return 0, TraceResult{}, err
	}
	return runProgram(entry, params, request)
}

func runProgram(entry *cachedEntry, params TokenParams, request RequestInput) (float64, TraceResult, error) {
	if entry.usesExchangeRate {
		if err := request.ExchangeRate.Validate(); err != nil {
			return 0, TraceResult{}, err
		}
	}
	trace := TraceResult{
		RequestRules: append([]RequestRuleTrace(nil), entry.requestRules...),
	}
	headers := normalizeHeaders(request.Headers)

	env := map[string]any{
		"p":     params.P,
		"c":     params.C,
		"len":   params.Len,
		"cr":    params.CR,
		"cc":    params.CC,
		"cc1h":  params.CC1h,
		"img":   params.Img,
		"img_o": params.ImgO,
		"ai":    params.AI,
		"ao":    params.AO,
		"tier": func(name string, value float64) float64 {
			trace.MatchedTier = name
			trace.Cost = value
			return value
		},
		requestRuleTraceFunction: func(ruleIndex int, matched bool, multiplier float64) float64 {
			if matched && ruleIndex >= 0 && ruleIndex < len(trace.RequestRules) {
				trace.RequestRules[ruleIndex].Matched = true
			}
			if matched {
				return multiplier
			}
			return 1
		},
		requestRuleTraceIntFunction: func(ruleIndex int, matched bool, multiplier int) int {
			if matched && ruleIndex >= 0 && ruleIndex < len(trace.RequestRules) {
				trace.RequestRules[ruleIndex].Matched = true
			}
			if matched {
				return multiplier
			}
			return 1
		},
		"header": func(key string) string {
			return headers[strings.ToLower(strings.TrimSpace(key))]
		},
		"param": func(path string) any {
			path = strings.TrimSpace(path)
			if path == "" || len(request.Body) == 0 {
				return nil
			}
			result := gjson.GetBytes(request.Body, path)
			if !result.Exists() {
				return nil
			}
			return result.Value()
		},
		"u": func(name string) any {
			if request.Usage == nil {
				return nil
			}
			return request.Usage[strings.TrimSpace(name)]
		},
		"has": func(source any, substr string) bool {
			if source == nil || substr == "" {
				return false
			}
			return strings.Contains(fmt.Sprint(source), substr)
		},
		"hour":    func(tz string) int { return pricingTimeInZone(request.PricingTime, tz).Hour() },
		"minute":  func(tz string) int { return pricingTimeInZone(request.PricingTime, tz).Minute() },
		"weekday": func(tz string) int { return int(pricingTimeInZone(request.PricingTime, tz).Weekday()) },
		"month":   func(tz string) int { return int(pricingTimeInZone(request.PricingTime, tz).Month()) },
		"day":     func(tz string) int { return pricingTimeInZone(request.PricingTime, tz).Day() },
		"max":     math.Max,
		"min":     math.Min,
		"abs":     math.Abs,
		"ceil":    math.Ceil,
		"floor":   math.Floor,

		// The frozen CNY/USD context comes from the caller. Without one the
		// call errors instead of falling back to any other value, so a rate
		// misconfiguration surfaces before funds move.
		UsdExchangeRateFunc: func() (float64, error) {
			if request.ExchangeRate == nil {
				return 0, fmt.Errorf("%s() has no frozen exchange-rate context for this billing run", UsdExchangeRateFunc)
			}
			if err := request.ExchangeRate.Validate(); err != nil {
				return 0, err
			}
			return request.ExchangeRate.Rate, nil
		},
	}

	program := entry.prog
	if request.RecordCalculation {
		entry.calculationOnce.Do(func() {
			entry.calculationProgram, entry.calculationNodes, entry.calculationError = compileCalculation(entry.body, entry.version)
		})
		if entry.calculationError != nil {
			return 0, trace, calculationError{cause: entry.calculationError, phase: "compile"}
		}
		trace.Calculation = NewCalculation()
		trace.Calculation.ExpressionVersion = entry.version
		trace.Calculation.Nodes = entry.calculationNodes
		env["$billing_calculation"] = trace.Calculation
		program = entry.calculationProgram
	}
	out, err := expr.Run(program, env)
	trace.Calculation.protectValues()
	if err != nil {
		if request.RecordCalculation {
			return 0, trace, calculationError{cause: err, phase: "run"}
		}
		return 0, trace, fmt.Errorf("expr run error: %w", err)
	}
	f, ok := out.(float64)
	if !ok {
		return 0, trace, fmt.Errorf("expr result is %T, want float64", out)
	}
	if trace.Calculation != nil {
		// Tier names are taken only from static literals, never a request value.
		if entry.staticTiers[trace.MatchedTier] {
			trace.Calculation.MatchedTier = trace.MatchedTier
		}
		trace.Calculation.UsageFacts = make(map[string]any)
		for key, value := range request.Usage {
			if calculationValue(value) != "protected" {
				trace.Calculation.UsageFacts[key] = value
			}
		}
		for _, rule := range trace.RequestRules {
			rule.Cond = "protected condition"
			trace.Calculation.RequestRules = append(trace.Calculation.RequestRules, rule)
		}
	}
	return f, trace, nil
}

// pricingTimeInZone resolves the evaluation instant for the time functions.
// An explicit frozen pricing time always wins; without one the functions keep
// reading the current time, preserving native call semantics.
func pricingTimeInZone(pricingTime *time.Time, tz string) time.Time {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		if pricingTime != nil {
			return pricingTime.UTC()
		}
		return time.Now().UTC()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		if pricingTime != nil {
			return pricingTime.UTC()
		}
		return time.Now().UTC()
	}
	if pricingTime != nil {
		return pricingTime.In(loc)
	}
	return time.Now().In(loc)
}

func normalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		k := strings.ToLower(strings.TrimSpace(key))
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		normalized[k] = v
	}
	return normalized
}
