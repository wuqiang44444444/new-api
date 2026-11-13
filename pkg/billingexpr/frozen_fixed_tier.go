package billingexpr

import (
	"fmt"
	"math"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"
)

type frozenFixedTierKey struct {
	tier  string
	price float64
}

const frozenTierConditionFunction = "_frozen_tier_condition"

func frozenFixedTierProgram(entry *cachedEntry, tier string, price float64) (*vm.Program, error) {
	if tier == "" || price < 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return nil, fmt.Errorf("invalid frozen fixed tier")
	}
	key := frozenFixedTierKey{tier: tier, price: price}
	entry.frozenFixedMu.Lock()
	defer entry.frozenFixedMu.Unlock()
	if program := entry.frozenFixedTiers[key]; program != nil {
		return program, nil
	}
	// Keep the writer's original rule indices. Pruning before instrumentation
	// would shift the indices of rules that followed the discarded branches.
	selector := &frozenFixedTierSelector{key: key, rules: requestRulePatcher{freezeConditions: true}}
	program, err := expr.Compile(entry.body, expr.Env(getCompileEnv(entry.version)),
		expr.Patch(frozenFixedTierRulePatcher{selector}), expr.Patch(selector), expr.AsFloat64())
	if selector.err != nil {
		return nil, selector.err
	}
	if err != nil {
		return nil, err
	}
	// Only successful matches against literal leaves occupy the cache.
	if entry.frozenFixedTiers == nil {
		entry.frozenFixedTiers = make(map[frozenFixedTierKey]*vm.Program)
	}
	entry.frozenFixedTiers[key] = program
	return program, nil
}

type frozenFixedTierSelector struct {
	key   frozenFixedTierKey
	root  *ast.Node
	rules requestRulePatcher
	err   error
}

type frozenFixedTierRulePatcher struct{ selector *frozenFixedTierSelector }

func (p frozenFixedTierRulePatcher) Visit(node *ast.Node) {
	p.selector.rules.Visit(node)
	p.selector.root = node // The root is visited last.
}

func (s *frozenFixedTierSelector) Visit(node *ast.Node) {
	if node != s.root {
		return
	}
	shared := make(map[string]bool, len(s.rules.requestRules))
	for _, rule := range s.rules.requestRules {
		shared[rule.Cond] = true
	}
	selected, matches := selectFrozenFixedTier(*node, s.key, shared)
	if matches != 1 || hasUnfrozenRequestInput(selected) || ast.Find(selected, func(node ast.Node) bool {
		_, repeated := node.(*ast.PredicateNode)
		return repeated
	}) != nil {
		s.err = fmt.Errorf("frozen fixed tier does not uniquely establish the expression inputs")
		return
	}
	ast.Patch(node, selected)
}

// The validated fixed pricing grammar allows one pricing subtree in a
// product. Keep the product and its operands in their original order.
func selectFrozenFixedTier(node ast.Node, key frozenFixedTierKey, shared map[string]bool) (ast.Node, int) {
	switch n := node.(type) {
	case *ast.ConditionalNode:
		if condition, literal := n.Cond.(*ast.BoolNode); literal {
			if condition.Value {
				return selectFrozenFixedTier(n.Exp1, key, shared)
			}
			return selectFrozenFixedTier(n.Exp2, key, shared)
		}
		left, l := selectFrozenFixedTier(n.Exp1, key, shared)
		right, r := selectFrozenFixedTier(n.Exp2, key, shared)
		if l+r != 1 {
			return nil, l + r
		}
		predicate := n.Cond.String()
		condition, ok := frozenFixedTierCondition(n.Cond, l == 1)
		if !ok {
			return nil, 2
		}
		if shared[predicate] {
			condition = &ast.CallNode{
				Callee:    &ast.IdentifierNode{Value: frozenTierConditionFunction},
				Arguments: []ast.Node{condition, &ast.StringNode{Value: predicate}, &ast.BoolNode{Value: l == 1}},
			}
		}
		// The other branch cannot satisfy the final tier and unit checks.
		n.Cond, n.Exp1, n.Exp2 = condition, left, right
		if l == 0 {
			n.Exp1 = &ast.IntegerNode{Value: 0}
		}
		if r == 0 {
			n.Exp2 = &ast.IntegerNode{Value: 0}
		}
		return n, 1
	case *ast.BinaryNode:
		if n.Operator == "*" {
			if left, count := selectFrozenFixedTier(n.Left, key, shared); count != 0 {
				if count != 1 {
					return nil, count
				}
				n.Left = left
				return n, count
			}
			if right, count := selectFrozenFixedTier(n.Right, key, shared); count != 0 {
				if count != 1 {
					return nil, count
				}
				n.Right = right
				return n, count
			}
		}
	case *ast.CallNode:
		if !isFrozenPricingCall(n, "tier", 2) {
			break
		}
		label, ok := n.Arguments[0].(*ast.StringNode)
		fixed, isCall := n.Arguments[1].(*ast.CallNode)
		if !isCall || !isFrozenPricingCall(fixed, "fixed", 1) {
			break
		}
		if amount, literal := requestRuleNumber(fixed.Arguments[0]); !literal || amount != key.price {
			break
		}
		if !ok {
			// A dynamic label at the same price could be the recorded tier.
			return n, 2
		}
		if label.Value == key.tier {
			return n, 1
		}
	}
	return nil, 0
}

// Substitute an unknown request-only boolean in favor of the selected branch.
// The resulting condition is necessary for that branch: known inputs can
// still rule it out. Mixed atoms without a boolean split cannot be proved.
func frozenFixedTierCondition(node ast.Node, want bool) (ast.Node, bool) {
	if !hasUnfrozenRequestInput(node) {
		return node, true
	}
	switch n := node.(type) {
	case *ast.BinaryNode:
		switch n.Operator {
		case "&&", "and", "||", "or":
			left, leftOK := frozenFixedTierCondition(n.Left, want)
			right, rightOK := frozenFixedTierCondition(n.Right, want)
			if leftOK && rightOK {
				n.Left, n.Right = left, right
				return n, true
			}
			return nil, false
		}
	case *ast.UnaryNode:
		if n.Operator == "!" || n.Operator == "not" {
			child, ok := frozenFixedTierCondition(n.Node, !want)
			if ok {
				n.Node = child
			}
			return n, ok
		}
	}
	if ast.Find(node, func(part ast.Node) bool {
		id, ok := part.(*ast.IdentifierNode)
		if !ok {
			return false
		}
		switch id.Value {
		case "p", "c", "len", "cr", "cc", "cc1h", "img", "img_cr", "img_o", "ai", "ao", "image_count",
			UsdExchangeRateFunc, requestRuleTraceFunction, requestRuleTraceIntFunction, "$env", "u":
			return true
		}
		return false
	}) != nil {
		return nil, false
	}
	return &ast.BoolNode{Value: want}, true
}

func isFrozenPricingCall(call *ast.CallNode, name string, arity int) bool {
	id, ok := call.Callee.(*ast.IdentifierNode)
	return ok && id.Value == name && len(call.Arguments) == arity
}
