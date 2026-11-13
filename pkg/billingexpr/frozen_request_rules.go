package billingexpr

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

func requestMultiplierTrace(node *ast.ConditionalNode) (RequestRuleTrace, bool) {
	if !node.Ternary || !usesRequestProbe(node.Cond) {
		return RequestRuleTrace{}, false
	}
	multiplier, ok := requestRuleNumber(node.Exp1)
	fallback, fallbackOK := requestRuleNumber(node.Exp2)
	if !ok || !fallbackOK || fallback != 1 {
		return RequestRuleTrace{}, false
	}
	return RequestRuleTrace{Cond: node.Cond.String(), Multiplier: multiplier}, true
}

// WithFrozenRequestRules binds recorded outcomes to the exact original rule
// list. A unique frozen fixed-price leaf can additionally establish the branch
// when its selector depends on request inputs absent from the trace.
func WithFrozenRequestRules(expression string, request RequestInput, traces []RequestRuleTrace, tier string, fixedPrice *float64) (RequestInput, map[string]bool, error) {
	hash := ExprHashString(expression)
	entry, err := compileEntryFromCacheByHash(expression, hash)
	if err != nil {
		return RequestInput{}, nil, err
	}
	if len(traces) != len(entry.requestRules) {
		return RequestInput{}, nil, fmt.Errorf("frozen request rules do not match expression")
	}
	for i, rule := range entry.requestRules {
		if traces[i].Cond != rule.Cond || traces[i].Multiplier != rule.Multiplier {
			return RequestInput{}, nil, fmt.Errorf("frozen request rules do not match expression")
		}
	}
	entry.frozenRulesOnce.Do(func() {
		tree, err := parser.Parse(entry.body)
		if err != nil {
			entry.frozenRulesError = err
			return
		}
		// The writer records one matched flag per rule, not one result per
		// predicate iteration. A rule in a repeated evaluation is ambiguous.
		if ast.Find(tree.Node, func(node ast.Node) bool {
			_, repeated := node.(*ast.PredicateNode)
			return repeated
		}) != nil {
			entry.frozenRulesError = fmt.Errorf("expression can evaluate request rules more than once")
			return
		}
		ast.Walk(&tree.Node, removeTracedRequestProbes{})
		if hasUnfrozenRequestInput(tree.Node) {
			entry.frozenRulesError = fmt.Errorf("expression uses unrecorded request inputs")
			return
		}
		patcher := &requestRulePatcher{freezeConditions: true}
		entry.frozenRulesProgram, entry.frozenRulesError = expr.Compile(entry.body, expr.Env(getCompileEnv(entry.version)), expr.Patch(patcher), expr.AsFloat64())
	})
	if request.RecordCalculation {
		return RequestInput{}, nil, fmt.Errorf("frozen request rules cannot record a new calculation")
	}
	program := entry.frozenRulesProgram
	if entry.frozenRulesError != nil {
		if !entry.fixedPricing || fixedPrice == nil {
			return RequestInput{}, nil, entry.frozenRulesError
		}
		program, err = frozenFixedTierProgram(entry, tier, *fixedPrice)
		if err != nil {
			return RequestInput{}, nil, err
		}
	}
	request.frozenRuleProgram = program
	request.frozenRuleVars = extractUsedVars(program)
	request.frozenRuleHash = hash
	request.frozenRules = append([]RequestRuleTrace(nil), traces...)
	return request, request.frozenRuleVars, nil
}

func hasUnfrozenRequestInput(node ast.Node) bool {
	return usesRequestProbe(node) || ast.Find(node, func(node ast.Node) bool {
		id, ok := node.(*ast.IdentifierNode)
		return ok && (id.Value == "u" || id.Value == "$env")
	}) != nil
}

type removeTracedRequestProbes struct{}

func (removeTracedRequestProbes) Visit(node *ast.Node) {
	if conditional, ok := (*node).(*ast.ConditionalNode); ok {
		if _, traced := requestMultiplierTrace(conditional); traced {
			ast.Patch(node, &ast.IntegerNode{Value: 1})
		}
	}
}
