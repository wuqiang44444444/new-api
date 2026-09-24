package billingexpr

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/conf"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/vm"
)

type calculationError struct {
	cause error
	phase string
}

func (e calculationError) Error() string {
	message := "billing expression " + e.phase + " failed"
	var location *file.Error
	if errors.As(e.cause, &location) {
		message += fmt.Sprintf(" at line %d, column %d", location.Line, location.Column+1)
	}
	return message + " (request values protected)"
}
func (e calculationError) Unwrap() error { return e.cause }

// Calculation is evidence produced by the original evaluator, never executable
// pricing configuration. Decimal strings preserve the actual calculation precision.
// Request text and objects are never recorded; numeric containers retain operands.
type Calculation struct {
	ExpressionVersion int                `json:"expression_version,omitempty"`
	MatchedTier       string             `json:"matched_tier,omitempty"`
	UsageFacts        map[string]any     `json:"usage_facts,omitempty"`
	RequestRules      []RequestRuleTrace `json:"request_rules,omitempty"`
	Version           int                `json:"version"`
	Nodes             []CalculationNode  `json:"nodes,omitempty"`
	Values            []CalculationValue `json:"values,omitempty"`
	Steps             []CalculationStep  `json:"steps,omitempty"`
	Quota             int                `json:"quota"`
}

type CalculationNode struct {
	// Compile-time privacy metadata; never persisted or used for pricing.
	redact      bool
	boolean     bool
	bound       bool
	privacyArgs []int  // May contain accumulator back edges; never part of the public graph.
	ID          int    `json:"id"`
	Op          string `json:"op"`
	Args        []int  `json:"args,omitempty"`
	Literal     string `json:"literal,omitempty"`
}

type CalculationValue struct {
	Node  int    `json:"node"`
	Value string `json:"value"`
}

type CalculationStep struct {
	Formula string   `json:"formula"`
	Op      string   `json:"op"`
	Inputs  []string `json:"inputs,omitempty"`
	Result  string   `json:"result"`
	Unit    string   `json:"unit,omitempty"`
}

func NewCalculation() *Calculation { return &Calculation{Version: 1} }

// Add records values already computed by the owner. It does not calculate prices.
func (c *Calculation) Add(op, unit string, result any, inputs ...any) {
	if c == nil {
		return
	}
	s := CalculationStep{Op: op, Unit: unit, Result: CalculationNumber(result)}
	for _, v := range inputs {
		s.Inputs = append(s.Inputs, CalculationNumber(v))
	}
	s.Formula = recordedHostFormula(op, s.Inputs, s.Result)
	c.Steps = append(c.Steps, s)
}

func (c *Calculation) Finish(quota int) *Calculation {
	if c != nil {
		c.Quota = quota
	}
	return c
}

// CalculationNumber also accepts decimal.String() at host arithmetic boundaries.
func CalculationNumber(v any) string {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return "non_finite"
		}
		return strconv.FormatFloat(n, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(n), 'g', -1, 32)
	default:
		return fmt.Sprint(v)
	}
}

func calculationValue(v any) string {
	if v == nil {
		return "null"
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return CalculationNumber(v)
	case reflect.Array, reflect.Slice:
		value := reflect.ValueOf(v)
		items := make([]string, value.Len())
		for i := range items {
			items[i] = calculationValue(value.Index(i).Interface())
			if items[i] == "protected" {
				return "protected"
			}
		}
		// Numeric containers retain operands for aggregate functions. Arbitrary
		// objects and any container containing request text stay protected.
		return "[" + strings.Join(items, ", ") + "]"
	default:
		return "protected"
	}
}

// Callbacks are stateless compiled functions. The recorder is supplied by each
// run's environment, so the compile cache can never capture customer values.
func recordCalculationValue(args ...any) (any, error) {
	c := args[0].(*Calculation)
	value := calculationValue(args[2])
	if c.Nodes[args[1].(int)-1].redact {
		value = "protected"
	}
	c.Values = append(c.Values, CalculationValue{Node: args[1].(int), Value: value})
	return args[2], nil
}

type calculationPatcher struct {
	config    *conf.Config
	nodes     []CalculationNode
	ids       map[ast.Node]int
	functions map[reflect.Type]string
}

func (p *calculationPatcher) Visit(ptr *ast.Node) {
	n := *ptr
	d := CalculationNode{ID: len(p.nodes) + 1}
	wrap := true
	add := func(children ...ast.Node) {
		for _, child := range children {
			if id := p.ids[child]; id != 0 {
				d.Args = append(d.Args, id)
			}
		}
	}
	switch v := n.(type) {
	case *ast.IntegerNode:
		d.Op, d.Literal, wrap = "literal", strconv.Itoa(v.Value), false
	case *ast.FloatNode:
		d.Op, d.Literal, wrap = "literal", CalculationNumber(v.Value), false
	case *ast.BoolNode:
		d.Op, d.Literal, wrap = "literal", strconv.FormatBool(v.Value), false
	case *ast.NilNode:
		d.Op, d.Literal, wrap = "literal", "null", false
	case *ast.StringNode:
		d.Op, d.Literal, wrap = "literal", "protected", false
	case *ast.IdentifierNode:
		if n.Type().Kind() == reflect.Func {
			return
		}
		d.Op = "variable"
		switch v.Value {
		case "p", "c", "len", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao":
			d.Op = v.Value
		}
	case *ast.UnaryNode:
		d.Op = "unary:" + v.Operator
		add(v.Node)
	case *ast.BinaryNode:
		d.Op = v.Operator
		if v.Operator == "/" && n.Type().Kind() >= reflect.Int && n.Type().Kind() <= reflect.Uint64 {
			d.Op = "integer_division"
		}
		add(v.Left, v.Right)
	case *ast.ConditionalNode:
		d.Op = "if"
		add(v.Cond, v.Exp1, v.Exp2)
	case *ast.CallNode:
		d.Op = "call"
		if id, ok := v.Callee.(*ast.IdentifierNode); ok {
			if _, known := compileEnvPrototypeV1[id.Value]; known {
				d.Op = id.Value
				if id.Value == "u" && len(v.Arguments) == 1 {
					if key, ok := v.Arguments[0].(*ast.StringNode); ok {
						d.Op = "usage:" + key.Value
					}
				}
			}
		}
		if d.Op == "header" {
			d.redact = true
		}
		if (d.Op == "param" || d.Op == "call") && len(v.Arguments) == 1 && protectedCalculationKey(v.Arguments[0]) {
			// A function alias or $env member can still call param. Unknown
			// callees cannot make a sensitive/dynamic lookup key public.
			d.redact = true
			if d.Op == "param" {
				d.Op = "protected_request"
			}
		}
		add(v.Arguments...)
	case *ast.BuiltinNode:
		d.Op = v.Name
		add(v.Arguments...)
		for _, arg := range v.Arguments {
			if predicate, ok := arg.(*ast.PredicateNode); ok && len(v.Arguments) > 0 {
				binding := calculationBinding{patcher: p, value: p.ids[v.Arguments[0]], item: true}
				if v.Name == "reduce" {
					binding.accumulator = p.ids[predicate.Node]
					if len(v.Arguments) == 3 {
						binding.initial = p.ids[v.Arguments[2]]
					}
				}
				ast.Walk(&predicate.Node, &binding)
			}
		}
	case *ast.ArrayNode:
		d.Op = "array"
		add(v.Nodes...)
	case *ast.MapNode:
		d.Op = "map"
		add(v.Pairs...)
	case *ast.PairNode:
		d.Op, wrap = "pair", false
		add(v.Key, v.Value)
	case *ast.MemberNode:
		d.Op, wrap = "member", !v.Optional && !v.Method
		d.redact = protectedCalculationKey(v.Property)
		add(v.Node, v.Property)
	case *ast.ChainNode:
		d.Op = "optional"
		add(v.Node)
	case *ast.SliceNode:
		d.Op = "slice"
		add(v.Node, v.From, v.To)
	case *ast.PredicateNode:
		d.Op, wrap = "predicate", false
		add(v.Node)
	case *ast.PointerNode:
		d.Op = "item"
	case *ast.VariableDeclaratorNode:
		d.Op, wrap = "let", false
		add(v.Value, v.Expr)
		ast.Walk(&v.Expr, &calculationBinding{patcher: p, name: v.Name, value: p.ids[v.Value]})
	case *ast.SequenceNode:
		d.Op = "sequence"
		add(v.Nodes...)
	default:
		return
	}
	d.boolean = n.Type() != nil && n.Type().Kind() == reflect.Bool
	p.nodes = append(p.nodes, d)
	p.ids[n] = d.ID
	if !wrap {
		return
	}
	t := n.Type()
	name := p.functions[t]
	if name == "" {
		name = fmt.Sprintf("$billing_value_%d", len(p.functions))
		p.functions[t] = name
		signature := reflect.FuncOf([]reflect.Type{reflect.TypeOf((*Calculation)(nil)), reflect.TypeOf(0), t}, []reflect.Type{t}, false)
		expr.Function(name, recordCalculationValue, reflect.Zero(signature).Interface())(p.config)
	}
	ast.Patch(ptr, &ast.CallNode{Callee: &ast.IdentifierNode{Value: name}, Arguments: []ast.Node{
		&ast.IdentifierNode{Value: "$billing_calculation"}, &ast.IntegerNode{Value: d.ID}, n,
	}})
	p.ids[*ptr] = d.ID
}

func compileCalculation(body string, version int) (*vm.Program, []CalculationNode, error) {
	env := make(map[string]any)
	for k, v := range getCompileEnv(version) {
		env[k] = v
	}
	env["$billing_calculation"] = (*Calculation)(nil)
	p := &calculationPatcher{ids: make(map[ast.Node]int), functions: make(map[reflect.Type]string)}
	option := func(c *conf.Config) { p.config = c; c.Visitors = append(c.Visitors, p) }
	program, err := expr.Compile(body, expr.Env(env), expr.Patch(&requestRulePatcher{}), option, expr.AsFloat64())
	resolveCalculationPrivacy(p.nodes)
	return program, p.nodes, err
}

func expressionDivisor(taskUsage bool) float64 {
	if taskUsage {
		return 1
	}
	return 1000000
}

// RecordExpressionQuota appends the host conversion performed after evaluation.
func RecordExpressionQuota(c *Calculation, cost, beforeGroup, group, quotaPerUnit float64, taskUsage bool) {
	c.Add("quota_conversion", "quota", beforeGroup, cost, quotaPerUnit, expressionDivisor(taskUsage))
	c.Add("group_ratio", "quota", beforeGroup*group, beforeGroup, group)
}
