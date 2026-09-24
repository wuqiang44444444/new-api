package billingexpr

import (
	"strings"

	"github.com/expr-lang/expr/ast"
)

// A static pricing key can be inspected without preserving its request value.
// Dynamic keys cannot establish that the selected request field is public.
func protectedCalculationKey(n ast.Node) bool {
	key, ok := n.(*ast.StringNode)
	if !ok {
		_, numericIndex := n.(*ast.IntegerNode)
		return !numericIndex
	}
	// GJSON wildcards, queries and modifiers can select a sensitive field
	// without naming it. Only exact paths establish public numeric inputs.
	if strings.ContainsAny(key.Value, "*?#@|![]{}") {
		return true
	}
	// Treat camelCase, snake_case, dotted and dashed spellings consistently.
	normalized := strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.', '\\', ' ':
			return -1
		default:
			return r
		}
	}, strings.ToLower(key.Value))
	for _, sensitive := range []string{"authorization", "cookie", "password", "secret", "signature", "apikey", "accesstoken", "credential"} {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

// Bind already-visited local references to their value node for privacy only.
// Nested declarations/predicates bind first, so outer scopes cannot replace them.
type calculationBinding struct {
	patcher     *calculationPatcher
	name        string
	value       int
	item        bool
	accumulator int
	initial     int
}

func (b *calculationBinding) Visit(ptr *ast.Node) {
	id := b.patcher.ids[*ptr]
	if id == 0 || b.value == 0 || b.patcher.nodes[id-1].bound {
		return
	}
	node := &b.patcher.nodes[id-1]
	switch n := (*ptr).(type) {
	case *ast.IdentifierNode:
		if b.item || n.Value != b.name {
			return
		}
	case *ast.PointerNode:
		if !b.item {
			return
		}
		switch n.Name {
		case "index":
			// The iteration index does not contain the collection's values.
			node.bound = true
			return
		case "acc":
			// reduce carries its initial value and every prior predicate result.
			// Keep this cycle out of the persisted display graph.
			node.bound = true
			node.privacyArgs = []int{b.value, b.accumulator}
			if b.initial != 0 {
				node.privacyArgs = append(node.privacyArgs, b.initial)
			}
			return
		}
	default:
		return
	}
	node.bound = true
	node.Args = []int{b.value}
}

// Propagate protection through conversions and arithmetic, stopping at safe
// boolean decisions. Selected price branches depend on their operands, not on
// the protected condition that selected them. No request values are evaluated.
func resolveCalculationPrivacy(nodes []CalculationNode) {
	// Accumulators introduce back edges even though AST IDs are post-order.
	// Protection is monotonic; stop once the dependency closure is stable.
	for changed := true; changed; {
		changed = false
		for i := range nodes {
			n := &nodes[i]
			if n.redact || n.boolean {
				continue
			}
			args := n.Args
			if n.privacyArgs != nil {
				args = n.privacyArgs
			}
			switch n.Op {
			case "if":
				args = args[1:]
			case "let":
				args = args[len(args)-1:]
			case "_trace", "_trace_int":
				args = args[2:]
			}
			for _, child := range args {
				if nodes[child-1].redact {
					n.redact, changed = true, true
					break
				}
			}
		}
	}
}

// protectValues closes runtime source protection before any trace leaves the
// evaluator, including on errors. A param's dynamic type is not known at compile
// time: strings/objects are never retained, and their numeric derivatives must
// also be removed. Work on a local copy so cached programs stay request-free.
func (c *Calculation) protectValues() {
	if c == nil {
		return
	}
	var nodes []CalculationNode
	for _, value := range c.Values {
		if value.Value == "protected" && !c.Nodes[value.Node-1].redact {
			if nodes == nil {
				nodes = append([]CalculationNode(nil), c.Nodes...)
			}
			nodes[value.Node-1].redact = true
		}
	}
	if nodes == nil {
		return
	}
	resolveCalculationPrivacy(nodes)
	for i := range c.Values {
		if nodes[c.Values[i].Node-1].redact {
			c.Values[i].Value = "protected"
		}
	}
}
