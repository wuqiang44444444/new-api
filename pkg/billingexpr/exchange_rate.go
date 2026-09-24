package billingexpr

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// UsdExchangeRateFunc is the zero-argument expression function that returns
// the CNY/USD exchange rate of the current billing context. The function name
// is a published part of the expression language; expressions reference it
// exactly and the engine rejects aliases and dynamic calls at compile time.
const UsdExchangeRateFunc = "usd_exchange_rate"

// UsdExchangeRateSourceKey is the global option key whose persisted value
// backs usd_exchange_rate(). The engine never reads it directly; hosts
// resolve the option and freeze an ExchangeRateContext per billing run.
const UsdExchangeRateSourceKey = "USDExchangeRate"

// ExchangeRateContext is the immutable CNY/USD rate fact frozen for one
// billing evaluation. Rate is CNY per 1 USD. The engine receives this value
// from the caller and never queries the database, global variables or an
// external rate service, so the same expression with the same usage and the
// same frozen context always produces the same cost.
//
// The zero value is not a valid rate: a missing or invalid context is a
// billing configuration error, never a silent fallback to 1, the built-in
// default or the last successful value.
type ExchangeRateContext struct {
	// SourceKey names the persisted option this value was resolved from.
	SourceKey string `json:"source_key"`
	// Rate is the CNY amount equivalent to 1 USD (CNY/USD direction).
	Rate float64 `json:"rate"`
	// FrozenAt is the instant the host froze this rate for the billing run.
	FrozenAt time.Time `json:"frozen_at"`
}

// Validate reports whether the context can price a request. Rates that are
// not finite and strictly positive fail closed.
func (c *ExchangeRateContext) Validate() error {
	if c == nil {
		return errors.New("usd_exchange_rate context is missing")
	}
	if math.IsNaN(c.Rate) || math.IsInf(c.Rate, 0) {
		return fmt.Errorf("usd_exchange_rate %v is not a finite number", c.Rate)
	}
	if c.Rate <= 0 {
		return fmt.Errorf("usd_exchange_rate %v must be greater than zero", c.Rate)
	}
	return nil
}

// NewExchangeRateContext builds a validated context for one billing run.
func NewExchangeRateContext(rate float64, frozenAt time.Time) (*ExchangeRateContext, error) {
	ctx := &ExchangeRateContext{
		SourceKey: UsdExchangeRateSourceKey,
		Rate:      rate,
		FrozenAt:  frozenAt,
	}
	if err := ctx.Validate(); err != nil {
		return nil, err
	}
	return ctx, nil
}

// UsesExchangeRate reports whether the expression references
// usd_exchange_rate() anywhere, including branches the current input would
// not take. Detection walks the unoptimized AST; string containment is never
// used. Unsupported shapes (aliases, dynamic calls) fail at compile time
// before this check is reached.
func UsesExchangeRate(exprStr string) bool {
	if exprStr == "" {
		return false
	}
	hash := ExprHashString(exprStr)
	cacheMu.RLock()
	if entry, ok := cache[hash]; ok {
		cacheMu.RUnlock()
		return entry.usesExchangeRate
	}
	cacheMu.RUnlock()

	if _, err := compileFromCacheByHash(exprStr, hash); err != nil {
		return false
	}
	cacheMu.RLock()
	entry, ok := cache[hash]
	cacheMu.RUnlock()
	if !ok {
		return false
	}
	return entry.usesExchangeRate
}

// exchangeRateDependency validates references before optimization, so even an
// unreachable branch must use the published direct, zero-argument call.
func exchangeRateDependency(body string) (bool, error) {
	tree, err := parser.Parse(body)
	if err != nil {
		return false, err
	}
	allowed := make(map[*ast.IdentifierNode]bool)
	usesRate := false
	ast.Find(tree.Node, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallNode:
			if id, ok := n.Callee.(*ast.IdentifierNode); ok && id.Value == UsdExchangeRateFunc && len(n.Arguments) == 0 {
				allowed[id] = true
				usesRate = true
			}
		case *ast.MemberNode:
			// Static access to existing environment fields remains valid. Dynamic
			// access or passing the whole environment could hide a rate function.
			if id, ok := n.Node.(*ast.IdentifierNode); ok && id.Value == "$env" {
				if key, ok := n.Property.(*ast.StringNode); ok && key.Value != UsdExchangeRateFunc {
					allowed[id] = true
				}
			}
		}
		return false
	})
	invalid := ast.Find(tree.Node, func(node ast.Node) bool {
		id, ok := node.(*ast.IdentifierNode)
		return ok && (id.Value == UsdExchangeRateFunc || id.Value == "$env") && !allowed[id]
	})
	if invalid != nil {
		return false, fmt.Errorf("%s must be called directly with no arguments; aliases and dynamic environment access are unsupported", UsdExchangeRateFunc)
	}
	return usesRate, nil
}

// ValidateExchangeRateValue checks a raw configured rate: it must be finite
// and strictly positive. NaN, infinities and zero/negative values are billing
// configuration errors, never clamped into a usable fallback.
func ValidateExchangeRateValue(rate float64) error {
	c := &ExchangeRateContext{Rate: rate}
	return c.Validate()
}

// ParseExchangeRateFact rebuilds a frozen context from the minimal log fact
// written at settlement (source_key + rate + frozen_at). It returns nil when
// the fact is absent or invalid, so callers treat history as unprovable
// instead of repricing it with the current setting.
func ParseExchangeRateFact(fact map[string]any) *ExchangeRateContext {
	if fact == nil {
		return nil
	}
	rate, ok := fact["rate"].(float64)
	if !ok {
		return nil
	}
	ctx := &ExchangeRateContext{SourceKey: UsdExchangeRateSourceKey, Rate: rate}
	if ts, ok := fact["frozen_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			ctx.FrozenAt = parsed
		}
	}
	if err := ctx.Validate(); err != nil {
		return nil
	}
	return ctx
}
