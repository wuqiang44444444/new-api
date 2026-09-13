package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// convertLegacyExpression preserves every arithmetic operator, literal and
// condition in a proven scalar AST subset. Source edits change only c and
// exact frozen param keys into equivalent float/string/bool usage inputs.
// Scaling the whole result matches the legacy host's final division, including
// fixed fees and numeric thresholds. Do not serialize an optimized AST: that
// can reorder operations, erase float literal types or inject trace callbacks.
func convertLegacyExpression(expression string, schema map[string]jsplugin.UsageFieldSchema) (string, error) {
	if err := billingexpr.UnknownIdentifier(expression); err != nil {
		return "", err
	}
	if _, err := billingexpr.CompileFromCache(expression); err != nil {
		return "", err
	}
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return "", err
	}
	type replacement struct {
		from, to int
		text     string
	}
	edits := []replacement{}
	allowedFunctions := map[string]bool{"tier": true, "param": true, "max": true, "min": true, "abs": true, "ceil": true, "floor": true, "has": true}
	// A function identifier is valid only at a direct call site, never as a value.
	callees := map[*ast.IdentifierNode]bool{}
	var invalid error
	ast.Find(tree.Node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallNode)
		if !ok {
			return false
		}
		callee, ok := call.Callee.(*ast.IdentifierNode)
		if !ok || !allowedFunctions[callee.Value] {
			invalid = fmt.Errorf("indirect or unsupported function in migration")
			return false
		}
		callees[callee] = true
		if callee.Value == "param" {
			if len(call.Arguments) != 1 {
				invalid = fmt.Errorf("param must have one exact frozen path")
				return false
			}
			key, ok := call.Arguments[0].(*ast.StringNode)
			if !ok || !strings.HasPrefix(key.Value, "_task.") {
				invalid = fmt.Errorf("only literal frozen _task paths can migrate")
				return false
			}
			field := strings.TrimPrefix(key.Value, "_task.")
			if _, ok := schema[field]; !ok {
				invalid = fmt.Errorf("undeclared frozen field %q", field)
				return false
			}
			at := callee.Location()
			edits = append(edits, replacement{at.From, at.To, "u"})
			at = key.Location()
			edits = append(edits, replacement{at.From, at.To, strconv.Quote(field)})
		}
		return false
	})
	ast.Find(tree.Node, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.IdentifierNode:
			if callees[n] {
				return false
			}
			if n.Value != "c" {
				invalid = fmt.Errorf("identifier %q is outside the proven migration subset", n.Value)
				return false
			}
			at := n.Location()
			edits = append(edits, replacement{at.From, at.To, `u("tokens")`})
		case *ast.IntegerNode, *ast.FloatNode, *ast.StringNode, *ast.BoolNode, *ast.NilNode, *ast.UnaryNode, *ast.BinaryNode, *ast.ConditionalNode, *ast.CallNode:
		default:
			invalid = fmt.Errorf("expression construct %T requires manual migration", node)
		}
		return false
	})
	if invalid != nil {
		return "", invalid
	}
	// No legacy meter/probe means the text carries no evidence of its amount
	// unit. A constant may already be USD, including our own previous output.
	// Never silently divide such a price again; require explicit manual pricing.
	if len(edits) == 0 {
		return "", fmt.Errorf("constant price has no provable legacy unit; set its USD price explicitly")
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].from < edits[j].from })
	source := []rune(body)
	var result strings.Builder
	position := 0
	for _, edit := range edits {
		if edit.from < position || edit.to > len(source) {
			return "", fmt.Errorf("overlapping expression edits")
		}
		result.WriteString(string(source[position:edit.from]))
		result.WriteString(edit.text)
		position = edit.to
	}
	result.WriteString(string(source[position:]))
	candidate := "(" + result.String() + "\n) / 1000000"
	if _, err := billingexpr.CompileFromCache(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}
