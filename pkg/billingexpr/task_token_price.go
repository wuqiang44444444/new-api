package billingexpr

import (
	"fmt"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// ValidateTaskTokenPrice proves that the selected price is proportional to the
// measured tokens. Only frozen facts may select the price or set its rate, so
// evaluating again with actual usage cannot switch to a different tariff.
// Callers validate the typed usage fields and async determinism first.
func ValidateTaskTokenPrice(expression string, input RequestInput) error {
	_, body := ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return err
	}
	if ast.Find(tree.Node, func(candidate ast.Node) bool {
		builtin, ok := candidate.(*ast.BuiltinNode)
		return ok && builtin.Name == "now"
	}) != nil {
		return fmt.Errorf("draft price must not use the current time")
	}
	input.RecordCalculation = false
	if !isTaskTokenPrice(tree.Node, input) {
		return fmt.Errorf("draft price must multiply measured tokens by a frozen rate")
	}
	return nil
}

func isTaskTokenPrice(node ast.Node, input RequestInput) bool {
	if key, ok := usageCallField(node); ok {
		return key == "tokens"
	}
	switch n := node.(type) {
	case *ast.ConditionalNode:
		if usesTaskPriceTokens(n.Cond) {
			return false
		}
		selected, _, err := RunExprWithRequest("("+n.Cond.String()+") ? 1 : 0", TokenParams{}, input)
		if err != nil {
			return false
		}
		if selected == 1 {
			return isTaskTokenPrice(n.Exp1, input)
		}
		return isTaskTokenPrice(n.Exp2, input)
	case *ast.CallNode:
		return isFrozenPricingCall(n, "tier", 2) && !usesTaskPriceTokens(n.Arguments[0]) && isTaskTokenPrice(n.Arguments[1], input)
	case *ast.BinaryNode:
		switch n.Operator {
		case "*":
			if !usesTaskPriceTokens(n.Left) {
				return isTaskTokenPrice(n.Right, input)
			}
			return !usesTaskPriceTokens(n.Right) && isTaskTokenPrice(n.Left, input)
		case "/":
			return !usesTaskPriceTokens(n.Right) && isTaskTokenPrice(n.Left, input)
		}
	}
	return false
}

func usesTaskPriceTokens(node ast.Node) bool {
	return ast.Find(node, func(candidate ast.Node) bool {
		key, ok := usageCallField(candidate)
		return ok && key == "tokens"
	}) != nil
}
