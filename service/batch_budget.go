package service

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Budget bounds evaluate every price branch over the estimated token range.
// They are a reservation bound for those inputs, not a promise that local
// input token estimation equals the provider tokenizer. Unsupported arithmetic
// is rejected before funds are held; settlement still uses the Go engine.
type batchPriceRange struct{ low, high float64 }

func batchBudgetOutput(frozen *model.BatchFrozenSnapshot, line BatchLineEstimate) (float64, error) {
	_, body := billingexpr.ParseExprVersion(frozen.Expr)
	tree, err := parser.Parse(body)
	if err != nil {
		return 0, err
	}
	guard := &batchRequestProbeGuard{}
	ast.Walk(&tree.Node, guard)
	if guard.invalid {
		return 0, fmt.Errorf("Batch pricing request condition cannot be safely frozen")
	}
	bounds := map[string]batchPriceRange{
		"p": {0, float64(line.InputEst)}, "len": {0, float64(line.InputEst)}, "cr": {0, float64(line.InputEst)}, "c": {0, float64(line.OutputCap)},
	}
	value, err := batchBudgetRange(tree.Node, bounds)
	if err != nil {
		return 0, err
	}
	if math.IsInf(value.high, 0) || math.IsNaN(value.high) || value.high < 0 {
		return 0, fmt.Errorf("batch budget is invalid")
	}
	return value.high, nil
}

func batchBudgetRange(node ast.Node, bounds map[string]batchPriceRange) (batchPriceRange, error) {
	invalid := fmt.Errorf("batch expression cannot be safely budgeted; use token arithmetic and tier branches")
	switch n := node.(type) {
	case *ast.IntegerNode:
		v := float64(n.Value)
		return batchPriceRange{v, v}, nil
	case *ast.FloatNode:
		return batchPriceRange{n.Value, n.Value}, nil
	case *ast.IdentifierNode:
		if v, ok := bounds[n.Value]; ok {
			return v, nil
		}
		return batchPriceRange{}, invalid
	case *ast.ConditionalNode:
		a, err := batchBudgetRange(n.Exp1, bounds)
		if err != nil {
			return a, err
		}
		b, err := batchBudgetRange(n.Exp2, bounds)
		if err != nil {
			return b, err
		}
		return batchPriceRange{math.Min(a.low, b.low), math.Max(a.high, b.high)}, nil
	case *ast.UnaryNode:
		v, err := batchBudgetRange(n.Node, bounds)
		if err != nil {
			return v, err
		}
		if n.Operator == "-" {
			return batchPriceRange{-v.high, -v.low}, nil
		}
		if n.Operator == "+" {
			return v, nil
		}
		return v, invalid
	case *ast.CallNode:
		name, ok := n.Callee.(*ast.IdentifierNode)
		if ok && name.Value == "tier" && len(n.Arguments) == 2 {
			return batchBudgetRange(n.Arguments[1], bounds)
		}
		return batchPriceRange{}, invalid
	case *ast.BinaryNode:
		a, err := batchBudgetRange(n.Left, bounds)
		if err != nil {
			return a, err
		}
		b, err := batchBudgetRange(n.Right, bounds)
		if err != nil {
			return b, err
		}
		switch n.Operator {
		case "+":
			return batchPriceRange{a.low + b.low, a.high + b.high}, nil
		case "-":
			return batchPriceRange{a.low - b.high, a.high - b.low}, nil
		case "/":
			if b.low <= 0 && b.high >= 0 {
				return batchPriceRange{}, invalid
			}
			b = batchPriceRange{1 / b.high, 1 / b.low}
		case "*":
		default:
			return batchPriceRange{}, invalid
		}
		values := []float64{a.low * b.low, a.low * b.high, a.high * b.low, a.high * b.high}
		low, high := values[0], values[0]
		for _, v := range values[1:] {
			low = math.Min(low, v)
			high = math.Max(high, v)
		}
		return batchPriceRange{low, high}, nil
	}
	return batchPriceRange{}, invalid
}
