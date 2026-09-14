package billingexpr

import "github.com/expr-lang/expr/ast"

// taskPriceConditional lifts one conditional factor through arithmetic without
// evaluating it or changing the original AST. Each resulting leaf still passes
// the strict linear-price parser; branch/depth limits belong to its caller.
// This covers usage * (condition ? price : price), including nested rates,
// while leaving nonlinear usage and unsupported operations opaque.
func taskPriceConditional(node ast.Node) *ast.ConditionalNode {
	if conditional, ok := node.(*ast.ConditionalNode); ok && conditional.Ternary {
		return conditional
	}
	binary, ok := node.(*ast.BinaryNode)
	if !ok {
		return nil
	}
	switch binary.Operator {
	case "+", "-", "*", "/":
	default:
		return nil
	}
	if conditional := taskPriceConditional(binary.Left); conditional != nil {
		left, right := *binary, *binary
		left.Left, right.Left = conditional.Exp1, conditional.Exp2
		branch := *conditional
		branch.Exp1, branch.Exp2 = &left, &right
		return &branch
	}
	if conditional := taskPriceConditional(binary.Right); conditional != nil {
		left, right := *binary, *binary
		left.Right, right.Right = conditional.Exp1, conditional.Exp2
		branch := *conditional
		branch.Exp1, branch.Exp2 = &left, &right
		return &branch
	}
	return nil
}
