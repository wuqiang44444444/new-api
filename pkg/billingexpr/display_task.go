package billingexpr

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// 任务用量表达式的展示投影。金额合同为 USD：表达式结果是 USD，固定请求
// 费用为 USD，不做百万换算。展示单价按声明字段的单位合同规一化——token
// 单位字段以「USD / 每百万」表示，其余字段以「USD / 自身单位」表示，与
// 前端任务编辑器的单价约定一致。调用方必须传入已经确认的类型化字段事实
// （声明与单位），不能只靠表达式中是否出现 u() 判断单位；未声明字段上的
// u() 引用整体标记 opaque，绝不猜测字段含义。

// DisplayUnitTaskUsage 是任务 USD 表达式的展示单位合同。
const DisplayUnitTaskUsage = "usd_per_usage_unit"

// taskTokenDisplayScale 把 token 单位字段的原始系数换算为「USD / 每百万」，
// 与前端任务编辑器的单价约定保持一致；其他单位字段不做换算。
const taskTokenDisplayScale = 1_000_000

// TaskUsageFieldInfo 是投影调用方传入的一个声明字段事实：字段已声明，
// 且携带其展示单位（空表示枚举或布尔条件字段，不作为计量项展示）。
type TaskUsageFieldInfo struct {
	Unit string
}

var (
	taskDisplayCacheMu sync.RWMutex
	taskDisplayCache   = make(map[string]*DisplayProjection, 64)
)

// TaskDisplayProjectionFor 返回任务用量表达式在给定字段事实下的展示投影。
// 声明集合与单位都参与缓存键：同一表达式文本在不同字段上下文下不得复用
// 缓存结果。
func TaskDisplayProjectionFor(exprStr string, fields map[string]TaskUsageFieldInfo) (*DisplayProjection, error) {
	if strings.TrimSpace(exprStr) == "" {
		return nil, fmt.Errorf("empty billing expression")
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+":"+fields[name].Unit)
	}
	key := ExprHashString(exprStr + "\x00" + DisplayUnitTaskUsage + "\x00" + strings.Join(parts, ","))
	taskDisplayCacheMu.RLock()
	if cached, ok := taskDisplayCache[key]; ok {
		taskDisplayCacheMu.RUnlock()
		return cached, nil
	}
	taskDisplayCacheMu.RUnlock()
	projection, err := BuildTaskDisplayProjection(exprStr, fields)
	if err != nil {
		return nil, err
	}
	taskDisplayCacheMu.Lock()
	if len(taskDisplayCache) >= maxDisplayCacheSize {
		taskDisplayCache = make(map[string]*DisplayProjection, 64)
	}
	taskDisplayCache[key] = projection
	taskDisplayCacheMu.Unlock()
	return projection, nil
}

// TaskDisplayProjectionForWithRate 是带汇率上下文的任务投影入口。汇率数值
// 参与缓存键；rate 为 nil 且表达式依赖 usd_exchange_rate() 时，整体标记
// exchange_rate_unresolved。
func TaskDisplayProjectionForWithRate(exprStr string, fields map[string]TaskUsageFieldInfo, rate *ExchangeRateContext) (*DisplayProjection, error) {
	if strings.TrimSpace(exprStr) == "" {
		return nil, fmt.Errorf("empty billing expression")
	}
	if !UsesExchangeRate(exprStr) {
		return TaskDisplayProjectionFor(exprStr, fields)
	}
	if rate.Validate() != nil {
		return buildUnresolvedRateProjection(exprStr, DisplayUnitTaskUsage)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+":"+fields[name].Unit)
	}
	key := ExprHashString(exprStr + "\x00" + DisplayUnitTaskUsage + "\x00" + strings.Join(parts, ",") + "\x00exchange_rate\x00" + strconv.FormatFloat(rate.Rate, 'g', -1, 64))
	taskDisplayCacheMu.RLock()
	if cached, ok := taskDisplayCache[key]; ok {
		taskDisplayCacheMu.RUnlock()
		return projectionWithExchangeRate(cached, rate), nil
	}
	taskDisplayCacheMu.RUnlock()
	projection, err := BuildTaskDisplayProjectionWithRate(exprStr, fields, rate)
	if err != nil {
		return nil, err
	}
	projection.AppliedExchangeRate = nil
	taskDisplayCacheMu.Lock()
	if len(taskDisplayCache) >= maxDisplayCacheSize {
		taskDisplayCache = make(map[string]*DisplayProjection, 64)
	}
	taskDisplayCache[key] = projection
	taskDisplayCacheMu.Unlock()
	return projectionWithExchangeRate(projection, rate), nil
}

// BuildTaskDisplayProjection 解析原始任务表达式并产出严格投影。表达式无法
// 编译时返回错误；合法但不可证明的结构整体标记 opaque。
func BuildTaskDisplayProjection(exprStr string, fields map[string]TaskUsageFieldInfo) (*DisplayProjection, error) {
	return BuildTaskDisplayProjectionWithRate(exprStr, fields, nil)
}

// BuildTaskDisplayProjectionWithRate 以给定汇率上下文折叠 usd_exchange_rate()
// 常数子表达式。
func BuildTaskDisplayProjectionWithRate(exprStr string, fields map[string]TaskUsageFieldInfo, rate *ExchangeRateContext) (*DisplayProjection, error) {
	if strings.TrimSpace(exprStr) == "" {
		return nil, fmt.Errorf("empty billing expression")
	}
	if _, err := CompileFromCache(exprStr); err != nil {
		return nil, err
	}
	version, body := ParseExprVersion(exprStr)
	projection := &DisplayProjection{
		Unit:              DisplayUnitTaskUsage,
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
	if rate != nil && UsesExchangeRate(exprStr) {
		projection.AppliedExchangeRate = rate
	}
	builder := &taskDisplayBuilder{projection: projection, fields: fields, rate: rate}
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

// taskDisplayBuilder 单次任务投影构建状态；固定项不做百万换算，token 单位
// 字段的展示单价在档位内换算为「USD / 每百万」。
type taskDisplayBuilder struct {
	projection  *DisplayProjection
	reason      string
	constant    float64
	hasConstant bool
	fields      map[string]TaskUsageFieldInfo
	rate        *ExchangeRateContext
}

// evalConstant 把带上下文的 usd_exchange_rate() 折叠为常数。
func (b *taskDisplayBuilder) evalConstant(node ast.Node) (float64, bool) {
	if b.rate != nil {
		if call, ok := node.(*ast.CallNode); ok {
			if id, ok := call.Callee.(*ast.IdentifierNode); ok && id.Value == UsdExchangeRateFunc && len(call.Arguments) == 0 {
				return b.rate.Rate, true
			}
		}
	}
	return evalConstant(node)
}

func (b *taskDisplayBuilder) fail(reason string) bool {
	if b.reason == "" {
		b.reason = reason
	}
	b.projection.Reason = b.reason
	return false
}

func (b *taskDisplayBuilder) declared(field string) bool {
	_, ok := b.fields[field]
	return ok
}

// finish 在 exact 时填充聚合固定费用并展开单条条件倍率的两个场景。
func (b *taskDisplayBuilder) finish() bool {
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
		charge := b.constant
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

// usageCallField 提取 u("field") 的字面字段名；动态字段名不可投影。
func usageCallField(node ast.Node) (string, bool) {
	call, ok := node.(*ast.CallNode)
	if !ok {
		return "", false
	}
	callee, ok := call.Callee.(*ast.IdentifierNode)
	if !ok || callee.Value != "u" || len(call.Arguments) != 1 {
		return "", false
	}
	key, ok := call.Arguments[0].(*ast.StringNode)
	if !ok || key.Value == "" {
		return "", false
	}
	return key.Value, true
}

// usesUsageProbe 判断条件子树是否只引用已声明的 u() 字段。任何其他调用
// （param/header/时间函数等）或裸标识符都使条件不可证明。
func (b *taskDisplayBuilder) usesUsageProbe(node ast.Node) bool {
	proven := true
	ast.Find(node, func(current ast.Node) bool {
		if !proven {
			return false
		}
		switch value := current.(type) {
		case *ast.CallNode:
			callee, ok := value.Callee.(*ast.IdentifierNode)
			if !ok || callee.Value != "u" {
				proven = false
				return true
			}
			field, ok := usageCallField(value)
			if !ok || !b.declared(field) {
				proven = false
				return true
			}
			return false
		case *ast.IdentifierNode:
			if value.Value != "u" {
				proven = false
				return true
			}
		}
		return false
	})
	return proven
}

// walkExpression 分解顶层加减项；与通用投影一样只允许一个价格子树项。
func (b *taskDisplayBuilder) walkExpression(node ast.Node) bool {
	var terms []displayTerm
	if !flattenAdditive(node, &terms) {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	tierTerms := 0
	for _, term := range terms {
		if !b.walkTerm(term) {
			return false
		}
		if taskTermHasPriceSubtree(term.node) {
			tierTerms++
		}
	}
	if tierTerms > 1 {
		return b.fail(DisplayReasonUnsupportedShape)
	}
	return true
}

func taskTermHasPriceSubtree(node ast.Node) bool {
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

// walkTerm 处理一个顶层加减项：纯数字项是 USD 固定请求费用；价格子树项
// 只允许一个非数字因子；条件倍率要求条件只引用已声明 u() 字段。
func (b *taskDisplayBuilder) walkTerm(term displayTerm) bool {
	var factors []displayFactor
	if !flattenMultiplicative(term.node, &factors) {
		return b.fail(DisplayReasonUnrecognizedFactor)
	}
	scalar := float64(term.sign)
	rulesBefore := len(b.projection.Rules)
	var priceNode ast.Node
	for _, factor := range factors {
		if value, ok := b.evalConstant(factor.node); ok {
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
		if conditional, ok := factor.node.(*ast.ConditionalNode); ok && conditional.Ternary && b.usesUsageProbe(conditional.Cond) {
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
				multiplierRule := (&displayBuilder{projection: b.projection}).buildRule(conditional.Cond, multiplier, fallback, 0)
				b.projection.Rules = append(b.projection.Rules, multiplierRule)
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
		// 纯数字项：USD 固定请求费用，直接累计，不做百万换算。
		b.constant += scalar
		b.hasConstant = true
		return true
	}
	return b.walkPriceSubtree(priceNode, scalar)
}

// walkPriceSubtree 解释价格子树：单个 tier 调用或档位三元链。与通用投影
// 不同，条件成立分支允许继续嵌套条件树，两侧都递归展开，完整保留前序
// 条件否定。
func (b *taskDisplayBuilder) walkPriceSubtree(node ast.Node, scalar float64) bool {
	return b.collectTierChain(node, scalar, 0, nil)
}

func (b *taskDisplayBuilder) collectTierChain(node ast.Node, scalar float64, depth int, prior ast.Node) bool {
	if depth > maxDisplayDepth {
		return b.fail(DisplayReasonDisplayLimit)
	}
	if conditional, ok := node.(*ast.ConditionalNode); ok {
		if !conditional.Ternary {
			return b.fail(DisplayReasonUnsupportedShape)
		}
		cond := conditional.Cond
		if !b.collectTierChain(conditional.Exp1, scalar, depth+1, displayBranchCondition(prior, cond)) {
			return false
		}
		return b.collectTierChain(conditional.Exp2, scalar, depth+1, displayBranchCondition(prior, &ast.UnaryNode{Operator: "!", Node: cond}))
	}
	return b.addTier(node, prior, scalar)
}

// addTier 解释一个 tier("label", body) 调用为价格分支；单价按声明字段名
// 记录并按单位规一化（token 字段为 USD/每百万），固定项为 USD/请求。
// 正文内部的条件单价继续展开为独立档位，条件与档位条件合并。
func (b *taskDisplayBuilder) addTier(node ast.Node, condition ast.Node, scalar float64) bool {
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
	return b.addTierBodyBranch(call.Arguments[1], condition, label.Value, scalar, 0)
}

// addTierBodyBranch 递归展开 tier 正文中的条件单价树；叶子是线性金额而
// 不再是 tier 调用。深度超限整体失败，不截断后当作完整报价。
func (b *taskDisplayBuilder) addTierBodyBranch(node ast.Node, condition ast.Node, label string, scalar float64, depth int) bool {
	if depth > maxDisplayDepth {
		return b.fail(DisplayReasonDisplayLimit)
	}
	if conditional := taskPriceConditional(node); conditional != nil {
		cond := conditional.Cond
		if !b.usesUsageProbe(cond) {
			return b.fail(DisplayReasonUnsupportedShape)
		}
		if !b.addTierBodyBranch(conditional.Exp1, displayBranchCondition(condition, cond), label, scalar, depth+1) {
			return false
		}
		return b.addTierBodyBranch(conditional.Exp2, displayBranchCondition(condition, &ast.UnaryNode{Operator: "!", Node: cond}), label, scalar, depth+1)
	}
	return b.appendLinearTier(label, node, condition, scalar)
}

// appendLinearTier 解释一个线性金额叶子为档位；条件保留完整结构化树。
func (b *taskDisplayBuilder) appendLinearTier(label string, body ast.Node, condition ast.Node, scalar float64) bool {
	if len(b.projection.Tiers) >= maxDisplayTiers {
		return b.fail(DisplayReasonDisplayLimit)
	}
	coefficients, constant, hadConstant, ok := b.parseLinearTierBody(body, scalar)
	if !ok {
		return false
	}
	tier := DisplayTier{
		Label:       label,
		UnitPrices:  coefficients,
		Constant:    constant,
		HasConstant: hadConstant,
	}
	if condition != nil {
		tier.ConditionText = condition.String()
		rule := (&displayBuilder{projection: b.projection}).buildRule(condition, 1, 1, 0)
		tier.Condition = &rule
	}
	b.projection.Tiers = append(b.projection.Tiers, tier)
	return true
}

// taskLinearSum 在 linearSum 之上记录是否存在常数项（含显式零价）。
type taskLinearSum struct {
	sum         *linearSum
	hadConstant bool
}

func mergeTaskLinearSum(left, right taskLinearSum) taskLinearSum {
	return taskLinearSum{
		sum:         mergeLinearSum(left.sum, right.sum),
		hadConstant: left.hadConstant || right.hadConstant,
	}
}

// collectLinear 递归把 tier 正文收集为「u() 字段 × 系数 + 常数」的线性
// 组合；任何非线性结构（字段平方、字段分母、未声明字段、未知调用）整体
// 失败。显式零常数保留 hadConstant 标记。
func (b *taskDisplayBuilder) collectLinear(node ast.Node, scale float64) (taskLinearSum, bool) {
	if value, ok := b.evalConstant(node); ok {
		return taskLinearSum{
			sum:         &linearSum{coefficients: map[string]float64{}, constant: value * scale},
			hadConstant: true,
		}, true
	}
	switch n := node.(type) {
	case *ast.BinaryNode:
		switch n.Operator {
		case "+", "-":
			left, ok := b.collectLinear(n.Left, scale)
			if !ok {
				return taskLinearSum{}, false
			}
			factor := 1.0
			if n.Operator == "-" {
				factor = -1
			}
			right, ok := b.collectLinear(n.Right, scale*factor)
			if !ok {
				return taskLinearSum{}, false
			}
			return mergeTaskLinearSum(left, right), true
		case "*":
			if leftValue, ok := b.evalConstant(n.Left); ok {
				return b.collectLinear(n.Right, scale*leftValue)
			}
			if rightValue, ok := b.evalConstant(n.Right); ok {
				return b.collectLinear(n.Left, scale*rightValue)
			}
			return taskLinearSum{}, false
		case "/":
			if rightValue, ok := b.evalConstant(n.Right); ok {
				if rightValue == 0 {
					return taskLinearSum{}, false
				}
				return b.collectLinear(n.Left, scale/rightValue)
			}
			return taskLinearSum{}, false
		}
	case *ast.CallNode:
		if field, ok := usageCallField(n); ok && b.declared(field) {
			return taskLinearSum{
				sum: &linearSum{coefficients: map[string]float64{field: scale}},
			}, true
		}
	}
	return taskLinearSum{}, false
}

// parseLinearTierBody 把 tier 正文解释为「字段 × 常数系数 + 常数项」的线性
// 组合，并把 token 单位字段的展示单价换算为 USD/每百万；任何非线性结构
// 整体失败。
func (b *taskDisplayBuilder) parseLinearTierBody(node ast.Node, scalar float64) (map[string]float64, float64, bool, bool) {
	sum, ok := b.collectLinear(node, scalar)
	if !ok {
		return nil, 0, false, b.fail(DisplayReasonNonlinearPricing)
	}
	coefficients := sum.sum.coefficients
	for field, coefficient := range coefficients {
		if b.fields[field].Unit != "token" {
			continue
		}
		normalized := coefficient * taskTokenDisplayScale
		if !finiteDisplayNumber(normalized) {
			return nil, 0, false, b.fail(DisplayReasonUnsupportedShape)
		}
		coefficients[field] = normalized
	}
	return coefficients, sum.sum.constant, sum.hadConstant, true
}
