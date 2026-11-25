package billingexpr

import (
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
)

var errStorageHourlyRate = errors.New("invalid frozen hourly storage rate")

// StorageHourlyQuota evaluates the linear rate of a previously validated
// expression without converting currency to float before quota rounding.
func StorageHourlyQuota(expression, frozenTier string, frozenExchangeRate float64, quotaPerUSD decimal.Decimal, tokenHours int64, reserve bool) (int, error) {
	if tokenHours < 0 || !quotaPerUSD.IsPositive() {
		return 0, errStorageHourlyRate
	}
	rate, tier, err := storageHourlyExactRate(expression, frozenExchangeRate)
	if err != nil {
		return 0, err
	}
	if tier != frozenTier {
		return 0, errStorageHourlyRate
	}
	total := rate.Mul(rate, quotaPerUSD.Rat())
	total.Mul(total, big.NewRat(tokenHours, 1_000_000))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(total.Num(), total.Denom(), remainder)
	if (reserve && remainder.Sign() > 0) || (!reserve && remainder.Lsh(remainder, 1).Cmp(total.Denom()) >= 0) {
		quotient.Add(quotient, big.NewInt(1))
	}
	return common.QuotaFromDecimalStrict(decimal.NewFromBigInt(quotient, 0))
}

func storageHourlyExactRate(expression string, frozenExchangeRate float64) (*big.Rat, string, error) {
	version, body := ParseExprVersion(expression)
	if version != 1 || len(expression) > 1024 {
		return nil, "", errStorageHourlyRate
	}
	tree, err := parser.Parse(body)
	if err != nil {
		return nil, "", errStorageHourlyRate
	}
	source := []rune(body)
	var tier string
	var exchangeRate *big.Rat
	if frozenExchangeRate != 0 && ValidateExchangeRateValue(frozenExchangeRate) == nil {
		exchangeRate = decimal.NewFromFloat(frozenExchangeRate).Rat()
	}
	rate := evalStorageHourlyRate(tree.Node, source, &tier, exchangeRate)
	if rate == nil || rate.Sign() <= 0 {
		return nil, "", errStorageHourlyRate
	}
	return rate, tier, nil
}

// Validation has already proven linearity and a single tier. Substituting
// one million token-hours then gives the contractual unit rate directly.
func evalStorageHourlyRate(node ast.Node, source []rune, tier *string, exchangeRate *big.Rat) *big.Rat {
	switch n := node.(type) {
	case *ast.IntegerNode:
		return big.NewRat(int64(n.Value), 1)
	case *ast.FloatNode:
		location := n.Location()
		if location.From < 0 || location.To > len(source) || location.From >= location.To {
			return nil
		}
		literal := strings.ReplaceAll(string(source[location.From:location.To]), "_", "")
		if i := strings.IndexAny(literal, "eE"); i >= 0 {
			exponent, err := strconv.Atoi(literal[i+1:])
			if err != nil || exponent < -324 || exponent > 308 {
				return nil
			}
		}
		value, _ := new(big.Rat).SetString(literal)
		return value
	case *ast.CallNode:
		callee, ok := n.Callee.(*ast.IdentifierNode)
		if !ok {
			return nil
		}
		if callee.Value == "tier" && len(n.Arguments) == 2 {
			label, ok := n.Arguments[0].(*ast.StringNode)
			if !ok {
				return nil
			}
			*tier = label.Value
			return evalStorageHourlyRate(n.Arguments[1], source, tier, exchangeRate)
		}
		if callee.Value == "usd_exchange_rate" && len(n.Arguments) == 0 && exchangeRate != nil {
			return new(big.Rat).Set(exchangeRate)
		}
		if field, ok := usageCallField(n); ok && field == StorageTokenHours {
			return big.NewRat(1_000_000, 1)
		}
	case *ast.UnaryNode:
		value := evalStorageHourlyRate(n.Node, source, tier, exchangeRate)
		if value != nil && n.Operator == "-" {
			return value.Neg(value)
		}
		if n.Operator == "+" {
			return value
		}
	case *ast.BinaryNode:
		left := evalStorageHourlyRate(n.Left, source, tier, exchangeRate)
		right := evalStorageHourlyRate(n.Right, source, tier, exchangeRate)
		if left == nil || right == nil {
			return nil
		}
		switch n.Operator {
		case "+":
			return left.Add(left, right)
		case "-":
			return left.Sub(left, right)
		case "*":
			return left.Mul(left, right)
		case "/":
			if right.Sign() != 0 {
				return left.Quo(left, right)
			}
		}
	}
	return nil
}
