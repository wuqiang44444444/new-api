package billingexpr

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// 严格只读展示投影：把保存的完整表达式解释为可信的价格展示模型。
// 这里是展示语义的唯一权威，不参与真实计费；不可证明的结构整体标记为
// opaque，绝不输出猜测的局部价格。展示遍历使用未经 trace patch 的原始
// 语法树（先经 CompileFromCache 校验，再用同库 parser 解析原始 body）。

const (
	// DisplayStatusExact 表示整个金额结构都被解释。
	DisplayStatusExact = "exact"
	// DisplayStatusOpaque 表示存在无法证明的结构，隐藏不可信单价。
	DisplayStatusOpaque = "opaque"

	// DisplayUnitUSDPerMillionTokens 是通用 token 表达式的展示单位合同。
	DisplayUnitUSDPerMillionTokens = "usd_per_million_tokens"

	// DisplayProjectionVersion 随展示语义变化递增；消费者遇到不支持的
	// 版本必须按投影缺失处理，不得自行猜价。
	DisplayProjectionVersion = 2

	// 展示复杂度上限：只影响展示，不限制原生合法配置或实际计费。
	maxDisplayTiers     = 32
	maxDisplayRules     = 32
	maxDisplayDepth     = 16
	maxDisplayCacheSize = 256
)

// 稳定的 opaque 原因；文案由前端翻译，不暴露内部细节。
const (
	DisplayReasonUnsupportedShape    = "unsupported_shape"
	DisplayReasonNonlinearPricing    = "nonlinear_pricing"
	DisplayReasonUnrecognizedFactor  = "unrecognized_factor"
	DisplayReasonDisplayLimit        = "display_limit_exceeded"
	DisplayReasonVersionUnsupported  = "version_unsupported"
	DisplayReasonTaskUsageExpression = "task_usage_expression"
)

// token display vars accepted in tier bodies; anything outside this set
// cannot be projected into a trustworthy per-token unit price.
var displayTokenVars = map[string]bool{
	"p": true, "c": true, "len": true,
	"cr": true, "cc": true, "cc1h": true,
	"img": true, "img_o": true, "ai": true, "ao": true,
}

var displayTimeFuncs = map[string]bool{
	"hour": true, "minute": true, "weekday": true, "month": true, "day": true,
}

// DisplayCondition 是档位条件中的一条比较（完整分支条件的一部分）。
type DisplayCondition struct {
	Var   string  `json:"var"`
	Op    string  `json:"op"`
	Value float64 `json:"value"`
}

// DisplayTier 是一个已解释的价格分支：条件、档位名与已吸收固定乘除的
// 单价。UnitPrices 数值为 USD / 百万 tokens；Constant 是不随 token 缩放的
// 固定请求费用（USD）。
type DisplayTier struct {
	Label         string             `json:"label"`
	Conditions    []DisplayCondition `json:"conditions,omitempty"`
	ConditionText string             `json:"condition_text,omitempty"`
	Condition     *DisplayRule       `json:"condition,omitempty"`
	UnitPrices    map[string]float64 `json:"unit_prices"`
	Constant      float64            `json:"constant,omitempty"`
	HasConstant   bool               `json:"has_constant,omitempty"`
}

// DisplayRule 是静态条件倍率树。Text 始终是规范化条件原文；复合节点用
// Op/Children 表达 AND/OR/NOT 层级；叶子节点尽力结构化，解不开时
// TextOnly=true 只保留原文。Fallback 是条件为假时的倍率，非 1 时必须
// 保留，不能丢失另一半分支。
type DisplayRule struct {
	Text       string        `json:"text"`
	Multiplier float64       `json:"multiplier"`
	Fallback   float64       `json:"fallback"`
	Op         string        `json:"op,omitempty"`
	Children   []DisplayRule `json:"children,omitempty"`
	Source     string        `json:"source,omitempty"`
	TimeFunc   string        `json:"time_func,omitempty"`
	Timezone   string        `json:"timezone,omitempty"`
	CompareOp  string        `json:"compare_op,omitempty"`
	Value      string        `json:"value,omitempty"`
	Path       string        `json:"path,omitempty"`
	TextOnly   bool          `json:"text_only,omitempty"`
}

// DisplayScenario holds effective prices for one complete multiplier branch.
// Amounts are computed from the AST, never from a frontend clock.
type DisplayScenario struct {
	Matched bool          `json:"matched"`
	Tiers   []DisplayTier `json:"tiers"`
}

// DisplayProjection 是一份可重建的只读读模型，不是新的计费合同。
type DisplayProjection struct {
	Status            string            `json:"status"`
	Reason            string            `json:"reason,omitempty"`
	Unit              string            `json:"unit"`
	DisplayVersion    int               `json:"display_version"`
	ExpressionVersion int               `json:"expression_version"`
	ExpressionHash    string            `json:"expression_hash"`
	Tiers             []DisplayTier     `json:"tiers,omitempty"`
	Rules             []DisplayRule     `json:"rules,omitempty"`
	ConstantCharge    *float64          `json:"constant_charge,omitempty"`
	Scenarios         []DisplayScenario `json:"scenarios,omitempty"`
}

var (
	displayCacheMu sync.RWMutex
	displayCache   = make(map[string]*DisplayProjection, 64)
)

// DisplayProjectionFor 返回表达式的展示投影（按表达式 hash 有界缓存）。
// 只缓存静态、不可变的投影；表达式无法编译时返回错误，由调用方按请求
// 错误处理，不缓存失败结果。投影语义变化时由 DisplayProjectionVersion
// 表达，旧版本消费者不得当作 exact 使用。
func DisplayProjectionFor(exprStr string) (*DisplayProjection, error) {
	if strings.TrimSpace(exprStr) == "" {
		return nil, fmt.Errorf("empty billing expression")
	}
	hash := ExprHashString(exprStr)
	displayCacheMu.RLock()
	if cached, ok := displayCache[hash]; ok {
		displayCacheMu.RUnlock()
		return cached, nil
	}
	displayCacheMu.RUnlock()
	projection, err := BuildDisplayProjection(exprStr)
	if err != nil {
		return nil, err
	}
	displayCacheMu.Lock()
	if len(displayCache) >= maxDisplayCacheSize {
		displayCache = make(map[string]*DisplayProjection, 64)
	}
	displayCache[hash] = projection
	displayCacheMu.Unlock()
	return projection, nil
}

// BuildDisplayProjection 解析原始表达式并产出严格投影。表达式无法编译
// 时返回错误（语法无效是请求错误，不是合法的不可展开）。
func BuildDisplayProjection(exprStr string) (*DisplayProjection, error) {
	if strings.TrimSpace(exprStr) == "" {
		return nil, fmt.Errorf("empty billing expression")
	}
	if _, err := CompileFromCache(exprStr); err != nil {
		return nil, err
	}
	version, body := ParseExprVersion(exprStr)
	projection := &DisplayProjection{
		Unit:              DisplayUnitUSDPerMillionTokens,
		DisplayVersion:    DisplayProjectionVersion,
		ExpressionVersion: version,
		ExpressionHash:    ExprHashString(exprStr),
	}
	if version != 1 {
		projection.Status = DisplayStatusOpaque
		projection.Reason = DisplayReasonVersionUnsupported
		return projection, nil
	}
	tree, err := parser.Parse(body)
	if err != nil {
		// 编译已通过而解析失败属于意外；按不可展开处理而非请求错误。
		projection.Status = DisplayStatusOpaque
		projection.Reason = DisplayReasonUnsupportedShape
		return projection, nil
	}
	builder := &displayBuilder{projection: projection}
	if builder.walkExpression(tree.Node) && builder.finish() {
		projection.Status = DisplayStatusExact
	} else {
		projection.Status = DisplayStatusOpaque
		if projection.Reason == "" {
			projection.Reason = DisplayReasonUnsupportedShape
		}
		projection.Tiers = nil
		projection.Rules = nil
		projection.ConstantCharge = nil
		projection.Scenarios = nil
	}
	return projection, nil
}

// displayBuilder 单次投影构建状态；fail 短路并记录稳定原因。
type displayBuilder struct {
	projection  *DisplayProjection
	reason      string
	constant    float64
	hasConstant bool
}

func (b *displayBuilder) fail(reason string) bool {
	if b.reason == "" {
		b.reason = reason
	}
	b.projection.Reason = b.reason
	return false
}

// finish 在 exact 时填充聚合的固定请求费用。
func (b *displayBuilder) finish() bool {
	if !finiteDisplayNumber(b.constant) {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	for _, tier := range b.projection.Tiers {
		if !finiteDisplayNumber(tier.Constant) {
			return b.fail(DisplayReasonUnsupportedShape)
		}
		for _, price := range tier.UnitPrices {
			if !finiteDisplayNumber(price) {
				return b.fail(DisplayReasonUnsupportedShape)
			}
		}
	}
	if b.hasConstant {
		charge := b.constant / 1_000_000
		b.projection.ConstantCharge = &charge
	}
	if len(b.projection.Rules) == 1 {
		rule := b.projection.Rules[0]
		for _, matched := range []bool{false, true} {
			factor := rule.Fallback
			if matched {
				factor = rule.Multiplier
			}
			scenario := DisplayScenario{Matched: matched}
			for _, base := range b.projection.Tiers {
				tier := base
				tier.UnitPrices = make(map[string]float64, len(base.UnitPrices))
				for variable, price := range base.UnitPrices {
					value := price * factor
					if !finiteDisplayNumber(value) {
						return b.fail(DisplayReasonUnsupportedShape)
					}
					tier.UnitPrices[variable] = value
				}
				tier.Constant *= factor
				if !finiteDisplayNumber(tier.Constant) {
					return b.fail(DisplayReasonUnsupportedShape)
				}
				scenario.Tiers = append(scenario.Tiers, tier)
			}
			b.projection.Scenarios = append(b.projection.Scenarios, scenario)
		}
	}
	return true
}

func finiteDisplayNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// displayFactor 是乘除链中的一个因子。
type displayFactor struct {
	div  bool
	node ast.Node
}

// flattenMultiplicative 展开嵌套的 `*` / `/` 乘除链。
func flattenMultiplicative(node ast.Node, out *[]displayFactor) bool {
	if binary, ok := node.(*ast.BinaryNode); ok {
		switch binary.Operator {
		case "*":
			if !flattenMultiplicative(binary.Left, out) {
				return false
			}
			return flattenMultiplicative(binary.Right, out)
		case "/":
			if !flattenMultiplicative(binary.Left, out) {
				return false
			}
			*out = append(*out, displayFactor{div: true, node: binary.Right})
			return true
		}
	}
	*out = append(*out, displayFactor{node: node})
	return true
}

// displayTerm 是加减链中的一个带符号项。
type displayTerm struct {
	sign int
	node ast.Node
}

func flattenAdditive(node ast.Node, out *[]displayTerm) bool {
	if binary, ok := node.(*ast.BinaryNode); ok {
		if binary.Operator == "+" || binary.Operator == "-" {
			if !flattenAdditive(binary.Left, out) {
				return false
			}
			sign := 1
			if binary.Operator == "-" {
				sign = -1
			}
			*out = append(*out, displayTerm{sign: sign, node: binary.Right})
			return true
		}
	}
	*out = append(*out, displayTerm{sign: 1, node: node})
	return true
}

// numericLiteral 解析数字字面量（含一元正负号），遵循引擎的 float64 语义。
func numericLiteral(node ast.Node) (float64, bool) {
	switch value := node.(type) {
	case *ast.IntegerNode:
		return float64(value.Value), true
	case *ast.FloatNode:
		return value.Value, true
	case *ast.UnaryNode:
		if value.Operator != "-" && value.Operator != "+" {
			return 0, false
		}
		inner, ok := numericLiteral(value.Node)
		if !ok {
			return 0, false
		}
		if value.Operator == "-" {
			return -inner, true
		}
		return inner, true
	default:
		return 0, false
	}
}

// walkExpression 分解顶层结构：加减项、乘除因子、价格子树与条件倍率。
func (b *displayBuilder) walkExpression(node ast.Node) bool {
	if taskUsageCall(node) {
		return b.fail(DisplayReasonTaskUsageExpression)
	}
	var terms []displayTerm
	if !flattenAdditive(node, &terms) {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	tierTerms := 0
	for _, term := range terms {
		if !b.walkTerm(term) {
			return false
		}
		if b.termHasPriceSubtree(term.node) {
			tierTerms++
		}
	}
	if tierTerms > 1 {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	return true
}

func (b *displayBuilder) termHasPriceSubtree(node ast.Node) bool {
	var factors []displayFactor
	if !flattenMultiplicative(node, &factors) {
		return false
	}
	for _, factor := range factors {
		if _, ok := numericLiteral(factor.node); ok {
			continue
		}
		if _, ok := factor.node.(*ast.CallNode); ok {
			return true
		}
		if _, ok := factor.node.(*ast.ConditionalNode); ok {
			return true
		}
	}
	return false
}

// walkTerm 处理一个顶层加减项：纯数字项是固定请求费用；价格子树项必须
// 只含一个非数字因子，其余因子是数字标量或已登记的条件倍率。
func (b *displayBuilder) walkTerm(term displayTerm) bool {
	var factors []displayFactor
	if !flattenMultiplicative(term.node, &factors) {
		return b.fail(DisplayReasonUnrecognizedFactor)
	}
	scalar := float64(term.sign)
	rulesBefore := len(b.projection.Rules)
	var priceNode ast.Node
	for _, factor := range factors {
		if value, ok := evalConstant(factor.node); ok {
			if factor.div {
				if value == 0 {
					return b.fail(DisplayReasonUnrecognizedFactor)
				}
				scalar /= value
			} else {
				scalar *= value
			}
			continue
		}
		if conditional, ok := factor.node.(*ast.ConditionalNode); ok && conditional.Ternary && usesRequestProbe(conditional.Cond) {
			multiplier, multiOK := numericLiteral(conditional.Exp1)
			fallback, fallbackOK := numericLiteral(conditional.Exp2)
			if multiOK && fallbackOK {
				if factor.div {
					if multiplier == 0 || fallback == 0 {
						return b.fail(DisplayReasonUnrecognizedFactor)
					}
					multiplier, fallback = 1/multiplier, 1/fallback
				}
				if !finiteDisplayNumber(multiplier) || !finiteDisplayNumber(fallback) {
					return b.fail(DisplayReasonUnrecognizedFactor)
				}
				if len(b.projection.Rules) >= maxDisplayRules {
					return b.fail(DisplayReasonDisplayLimit)
				}
				b.projection.Rules = append(b.projection.Rules, b.buildRule(conditional.Cond, multiplier, fallback, 0))
				continue
			}
		}
		if factor.div || priceNode != nil {
			return b.fail(DisplayReasonUnrecognizedFactor)
		}
		priceNode = factor.node
	}
	if priceNode == nil {
		if len(b.projection.Rules) != rulesBefore {
			return b.fail(DisplayReasonUnsupportedShape)
		}
		// 纯数字项：固定请求费用，单独标注，不并入 token 单价。
		b.constant += scalar
		b.hasConstant = true
		return true
	}
	return b.walkPriceSubtree(priceNode, scalar)
}

// walkPriceSubtree 解释价格子树：单个 tier 调用，或档位三元链
// （cond ? tier(...) : 下一分支）。标量乘除传递到其覆盖的整个价格子树。
func (b *displayBuilder) walkPriceSubtree(node ast.Node, scalar float64) bool {
	return b.collectTierChain(node, scalar, 0, nil)
}

func (b *displayBuilder) collectTierChain(node ast.Node, scalar float64, depth int, prior ast.Node) bool {
	if depth > maxDisplayDepth {
		return b.fail(DisplayReasonDisplayLimit)
	}
	if conditional, ok := node.(*ast.ConditionalNode); ok {
		if !conditional.Ternary {
			return b.fail(DisplayReasonUnsupportedShape)
		}
		if !b.addTier(conditional.Exp1, displayBranchCondition(prior, conditional.Cond), scalar) {
			return false
		}
		return b.collectTierChain(conditional.Exp2, scalar, depth+1, displayBranchCondition(prior, &ast.UnaryNode{Operator: "!", Node: conditional.Cond}))
	}
	return b.addTier(node, prior, scalar)
}

// displayBranchCondition preserves preceding false branches in a ternary chain.
func displayBranchCondition(prior, current ast.Node) ast.Node {
	if prior == nil {
		return current
	}
	return &ast.BinaryNode{Operator: "&&", Left: prior, Right: current}
}

// addTier 解释一个 tier("label", body) 调用为价格分支。
func (b *displayBuilder) addTier(node ast.Node, condition ast.Node, scalar float64) bool {
	call, ok := node.(*ast.CallNode)
	if !ok {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	callee, ok := call.Callee.(*ast.IdentifierNode)
	if !ok || callee.Value != "tier" || len(call.Arguments) != 2 {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	label, ok := call.Arguments[0].(*ast.StringNode)
	if !ok {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	if len(b.projection.Tiers) >= maxDisplayTiers {
		return b.fail(DisplayReasonDisplayLimit)
	}
	prices, constant, hasConstant, ok := b.parseLinearTierBody(call.Arguments[1], scalar)
	if !ok {
		return false
	}
	tier := DisplayTier{
		Label:       label.Value,
		UnitPrices:  prices,
		Constant:    constant / 1_000_000,
		HasConstant: hasConstant,
	}
	if condition != nil {
		tier.ConditionText = condition.String()
		rule := b.buildRule(condition, 1, 1, 0)
		tier.Condition = &rule
		tier.Conditions = parseTierConditions(condition)
	}
	b.projection.Tiers = append(b.projection.Tiers, tier)
	return true
}

// linearSum 是线性组合的累加表示：变量系数与常数项。
type linearSum struct {
	coefficients map[string]float64
	constant     float64
}

func mergeLinearSum(left, right *linearSum) *linearSum {
	merged := &linearSum{coefficients: make(map[string]float64, len(left.coefficients)+len(right.coefficients))}
	for name, value := range left.coefficients {
		merged.coefficients[name] += value
	}
	for name, value := range right.coefficients {
		merged.coefficients[name] += value
	}
	merged.constant = left.constant + right.constant
	return merged
}

// evalConstant 用 Go float64 语义折叠纯常数子表达式（四则运算与字面量）；
// 出现任何变量或调用即失败。
func evalConstant(node ast.Node) (float64, bool) {
	switch value := node.(type) {
	case *ast.IntegerNode:
		return float64(value.Value), true
	case *ast.FloatNode:
		return value.Value, true
	case *ast.UnaryNode:
		if value.Operator != "-" && value.Operator != "+" {
			return 0, false
		}
		inner, ok := evalConstant(value.Node)
		if !ok {
			return 0, false
		}
		if value.Operator == "-" {
			return -inner, true
		}
		return inner, true
	case *ast.BinaryNode:
		left, leftOK := evalConstant(value.Left)
		if !leftOK {
			return 0, false
		}
		right, rightOK := evalConstant(value.Right)
		if !rightOK {
			return 0, false
		}
		switch value.Operator {
		case "+":
			return left + right, true
		case "-":
			return left - right, true
		case "*":
			return left * right, true
		case "/":
			if right == 0 {
				return 0, false
			}
			return left / right, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// collectLinear 递归把表达式子树收集为「变量 × 系数 + 常数」的线性组合。
// 同一变量出现多次正确合并；乘除括号和式、常数折叠都得到处理；任何
// 非线性结构（变量平方、变量分母、未知调用）整体失败。
func (b *displayBuilder) collectLinear(node ast.Node, scale float64) (*linearSum, bool) {
	if value, ok := evalConstant(node); ok {
		return &linearSum{coefficients: map[string]float64{}, constant: value * scale}, true
	}
	switch n := node.(type) {
	case *ast.BinaryNode:
		switch n.Operator {
		case "+", "-":
			left, ok := b.collectLinear(n.Left, scale)
			if !ok {
				return nil, false
			}
			factor := 1.0
			if n.Operator == "-" {
				factor = -1
			}
			right, ok := b.collectLinear(n.Right, scale*factor)
			if !ok {
				return nil, false
			}
			return mergeLinearSum(left, right), true
		case "*":
			if leftValue, ok := evalConstant(n.Left); ok {
				return b.collectLinear(n.Right, scale*leftValue)
			}
			if rightValue, ok := evalConstant(n.Right); ok {
				return b.collectLinear(n.Left, scale*rightValue)
			}
			return nil, false
		case "/":
			if rightValue, ok := evalConstant(n.Right); ok {
				if rightValue == 0 {
					return nil, false
				}
				return b.collectLinear(n.Left, scale/rightValue)
			}
			return nil, false
		}
	case *ast.UnaryNode:
		if n.Operator == "-" {
			return b.collectLinear(n.Node, -scale)
		}
		if n.Operator == "+" {
			return b.collectLinear(n.Node, scale)
		}
	case *ast.IdentifierNode:
		if displayTokenVars[n.Value] {
			coefficient := scale
			return &linearSum{coefficients: map[string]float64{n.Value: coefficient}}, true
		}
	}
	return nil, false
}

// parseLinearTierBody 把 tier 正文解释为「变量 × 常数系数 + 常数项」的
// 线性组合；任何非线性结构整体失败。
func (b *displayBuilder) parseLinearTierBody(node ast.Node, scalar float64) (map[string]float64, float64, bool, bool) {
	sum, ok := b.collectLinear(node, scalar)
	if !ok {
		return nil, 0, false, b.fail(DisplayReasonNonlinearPricing)
	}
	return sum.coefficients, sum.constant, sum.constant != 0, true
}

// displayIdentifier 解析变量因子，支持一元负号（-p * 2 这类写法）。
func displayIdentifier(node ast.Node) (name string, negative bool, ok bool) {
	if identifier, isIdentifier := node.(*ast.IdentifierNode); isIdentifier {
		return identifier.Value, false, true
	}
	if unary, isUnary := node.(*ast.UnaryNode); isUnary && (unary.Operator == "-" || unary.Operator == "+") {
		if identifier, isIdentifier := unary.Node.(*ast.IdentifierNode); isIdentifier {
			return identifier.Value, unary.Operator == "-", true
		}
	}
	return "", false, false
}

// buildRule 把条件倍率的条件子树投影为完整布尔层级。叶子尽力结构化，
// 解不开时保留规范化原文并标记 text_only，绝不静默丢弃条件。
func (b *displayBuilder) buildRule(cond ast.Node, multiplier, fallback float64, depth int) DisplayRule {
	text := cond.String()
	rule := DisplayRule{Text: text, Multiplier: multiplier, Fallback: fallback}
	if depth > maxDisplayDepth {
		rule.TextOnly = true
		rule.Source = "text"
		return rule
	}
	switch node := cond.(type) {
	case *ast.BinaryNode:
		switch node.Operator {
		case "&&":
			rule.Op = "and"
			rule.Children = []DisplayRule{
				b.buildRule(node.Left, multiplier, fallback, depth+1),
				b.buildRule(node.Right, multiplier, fallback, depth+1),
			}
			return rule
		case "||":
			rule.Op = "or"
			rule.Children = []DisplayRule{
				b.buildRule(node.Left, multiplier, fallback, depth+1),
				b.buildRule(node.Right, multiplier, fallback, depth+1),
			}
			return rule
		}
		return b.buildRuleLeaf(cond, rule)
	case *ast.UnaryNode:
		if node.Operator == "!" || node.Operator == "not" {
			rule.Op = "not"
			rule.Children = []DisplayRule{b.buildRule(node.Node, multiplier, fallback, depth+1)}
			return rule
		}
	}
	return b.buildRuleLeaf(cond, rule)
}

// buildRuleLeaf 提取叶子条件的结构化事实（时间函数、param/header 比较、
// contains 与 exists）；解不开时仅保留原文。
func (b *displayBuilder) buildRuleLeaf(cond ast.Node, rule DisplayRule) DisplayRule {
	rule.TextOnly = true
	rule.Source = "text"
	// 任务合同的裸布尔字段条件 u("f"):结构化为 usage 字段等价 true。
	if field, ok := usageCallField(cond); ok {
		rule.TextOnly = false
		rule.Source = "usage"
		rule.Path = field
		rule.CompareOp = "=="
		rule.Value = "true"
		return rule
	}
	binary, isBinary := cond.(*ast.BinaryNode)
	if isBinary {
		switch binary.Operator {
		case "==", "!=", "<", "<=", ">", ">=":
			if source, path, timeFunc, timezone, ok := describeRuleSubject(binary.Left); ok {
				if value, valueOK := describeRuleValue(binary.Right); valueOK {
					rule.TextOnly = false
					rule.Source = source
					rule.Path = path
					rule.TimeFunc = timeFunc
					rule.Timezone = timezone
					rule.CompareOp = binary.Operator
					rule.Value = value
					return rule
				}
			}
		}
	}
	// has(header("h"), "v") / has(param("p"), "v") 是 contains 判断。
	if call, isCall := cond.(*ast.CallNode); isCall {
		if callee, calleeOK := call.Callee.(*ast.IdentifierNode); calleeOK && callee.Value == "has" && len(call.Arguments) == 2 {
			if source, path, _, _, ok := describeRuleSubject(call.Arguments[0]); ok && source != "time" {
				if value, valueOK := describeRuleValue(call.Arguments[1]); valueOK {
					rule.TextOnly = false
					rule.Source = source
					rule.Path = path
					rule.CompareOp = "contains"
					rule.Value = value
					return rule
				}
			}
		}
	}
	return rule
}

// describeRuleSubject 提取条件主语：时间函数（含时区）或 param/header 路径。
func describeRuleSubject(node ast.Node) (source, path, timeFunc, timezone string, ok bool) {
	if identifier, valid := node.(*ast.IdentifierNode); valid && displayTokenVars[identifier.Value] {
		return "token", identifier.Value, "", "", true
	}
	call, isCall := node.(*ast.CallNode)
	if !isCall {
		return "", "", "", "", false
	}
	callee, calleeOK := call.Callee.(*ast.IdentifierNode)
	if !calleeOK || len(call.Arguments) != 1 {
		return "", "", "", "", false
	}
	argument, argOK := call.Arguments[0].(*ast.StringNode)
	if !argOK {
		return "", "", "", "", false
	}
	if displayTimeFuncs[callee.Value] {
		return "time", "", callee.Value, argument.Value, true
	}
	if callee.Value == "param" {
		return "param", argument.Value, "", "", true
	}
	if callee.Value == "header" {
		return "header", argument.Value, "", "", true
	}
	if callee.Value == "u" {
		// 任务合同的条件主语：已声明用量字段，路径即字段名。
		return "usage", argument.Value, "", "", true
	}
	return "", "", "", "", false
}

// describeRuleValue 把条件宾语序列化为稳定文本（数值、字符串或布尔）。
func describeRuleValue(node ast.Node) (string, bool) {
	switch value := node.(type) {
	case *ast.IntegerNode:
		return fmt.Sprintf("%d", value.Value), true
	case *ast.FloatNode:
		return fmt.Sprintf("%v", value.Value), true
	case *ast.StringNode:
		return value.Value, true
	case *ast.BoolNode:
		if value.Value {
			return "true", true
		}
		return "false", true
	case *ast.NilNode:
		return "nil", true
	default:
		return "", false
	}
}

// parseTierConditions 解析档位条件链：AND 连接的「token 变量 比较 数字」
// 结构。任何不符合的局部使整组结构化失败，只保留 ConditionText。
func parseTierConditions(node ast.Node) []DisplayCondition {
	var comparisons []DisplayCondition
	if binary, ok := node.(*ast.BinaryNode); ok && binary.Operator == "&&" {
		comparisons = parseTierConditions(binary.Left)
		if comparisons == nil {
			return nil
		}
		next := parseTierConditions(binary.Right)
		if next == nil {
			return nil
		}
		return append(comparisons, next...)
	}
	comparison, ok := parseTierComparison(node)
	if !ok {
		return nil
	}
	return []DisplayCondition{comparison}
}

func parseTierComparison(node ast.Node) (DisplayCondition, bool) {
	binary, ok := node.(*ast.BinaryNode)
	if !ok {
		return DisplayCondition{}, false
	}
	switch binary.Operator {
	case "<", "<=", ">", ">=", "==":
	default:
		return DisplayCondition{}, false
	}
	left, negative, leftOK := displayIdentifier(binary.Left)
	if negative || !leftOK || !displayTokenVars[left] {
		return DisplayCondition{}, false
	}
	value, valueOK := numericLiteral(binary.Right)
	if !valueOK {
		return DisplayCondition{}, false
	}
	return DisplayCondition{Var: left, Op: binary.Operator, Value: value}, true
}

// taskUsageCall 检测表达式中是否包含 u("...") 任务用量调用；该类表达式
// 的单位合同不同，通用 token 投影必须整体标记 opaque。
func taskUsageCall(node ast.Node) bool {
	found := false
	ast.Find(node, func(current ast.Node) bool {
		if found {
			return false
		}
		call, ok := current.(*ast.CallNode)
		if !ok {
			return false
		}
		if callee, calleeOK := call.Callee.(*ast.IdentifierNode); calleeOK && callee.Value == "u" {
			found = true
			return true
		}
		return false
	})
	return found
}
