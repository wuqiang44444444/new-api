package billingexpr

import "github.com/expr-lang/expr/ast"

const imagePriceQuantityFunction = "_image_price_quantity"

// Fixed pricing permits image_count as a direct multiplier of a pricing
// subtree. Record it only when that multiplication actually executes.
func traceImagePriceQuantity(node *ast.Node) {
	product, ok := (*node).(*ast.BinaryNode)
	if !ok || product.Operator != "*" {
		return
	}
	operand, pricing := &product.Left, product.Right
	if id, ok := (*operand).(*ast.IdentifierNode); !ok || id.Value != "image_count" {
		operand, pricing = &product.Right, product.Left
	}
	if id, ok := (*operand).(*ast.IdentifierNode); ok && id.Value == "image_count" && containsPricingMarker(pricing) {
		ast.Patch(operand, &ast.CallNode{
			Callee:    &ast.IdentifierNode{Value: imagePriceQuantityFunction},
			Arguments: []ast.Node{*operand},
		})
	}
}
