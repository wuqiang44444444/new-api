package model

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

// Read each original object before JSON decoding can collapse duplicate keys.
// Snapshot facts can be recorded directly or under admin_info.
func hasDuplicateFrozenRequestRules(raw string) bool {
	if !strings.Contains(raw, "request_rules") && !strings.Contains(raw, `\`) {
		return false
	}
	var duplicate bool
	checkRules := func(object gjson.Result) {
		var seenRules bool
		object.ForEach(func(key, value gjson.Result) bool {
			if key.String() == "request_rules" {
				duplicate = seenRules
				seenRules = true
			}
			return !duplicate
		})
	}
	root := gjson.Parse(raw)
	checkRules(root)
	if duplicate {
		return true
	}
	root.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "statement_snapshot":
			checkRules(value)
		case "admin_info":
			value.ForEach(func(key, snapshot gjson.Result) bool {
				if key.String() == "statement_snapshot" {
					checkRules(snapshot)
				}
				return !duplicate
			})
		}
		return !duplicate
	})
	return duplicate
}

func frozenStatementRequestRules(raw json.RawMessage) ([]billingexpr.RequestRuleTrace, bool) {
	var entries []json.RawMessage
	if common.Unmarshal(raw, &entries) != nil || entries == nil {
		return nil, false
	}
	traces := make([]billingexpr.RequestRuleTrace, 0, len(entries))
	for _, entry := range entries {
		// Iterate the original object because decoding into a struct or map
		// would silently overwrite an earlier occurrence of a field.
		fields := make(map[string]string, 3)
		valid := true
		object := gjson.ParseBytes(entry)
		if !object.IsObject() {
			return nil, false
		}
		object.ForEach(func(key, value gjson.Result) bool {
			name := strings.ToLower(key.String())
			switch name {
			case "cond", "multiplier", "matched":
				_, duplicate := fields[name]
				valid = !duplicate && value.Type != gjson.Null
				fields[name] = value.Raw
			}
			return valid
		})
		var trace billingexpr.RequestRuleTrace
		if !valid || common.UnmarshalJsonStr(fields["cond"], &trace.Cond) != nil || trace.Cond == "" ||
			common.UnmarshalJsonStr(fields["multiplier"], &trace.Multiplier) != nil ||
			common.UnmarshalJsonStr(fields["matched"], &trace.Matched) != nil {
			return nil, false
		}
		traces = append(traces, trace)
	}
	return traces, true
}

func historicalBillingProbe(node ast.Node) bool {
	id, ok := node.(*ast.IdentifierNode)
	if !ok {
		return false
	}
	switch id.Value {
	case "param", "header", "hour", "minute", "weekday", "month", "day", "u":
		return true
	}
	return false
}

func historicalStatementTierExpression(expression, tier string) (string, bool) {
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return "", false
	}
	var leaves []*ast.CallNode
	if !collectHistoricalStatementTiers(tree.Node, &leaves) {
		return "", false
	}
	var selected *ast.CallNode
	for _, leaf := range leaves {
		if leaf.Arguments[0].(*ast.StringNode).Value != tier {
			continue
		}
		price, ok := leaf.Arguments[1].(*ast.CallNode)
		if ok && historicalStatementCall(price, "fixed", 1) {
			continue
		}
		if selected != nil {
			return "", false
		}
		selected = leaf
	}
	if selected == nil {
		return "", false
	}
	// A unique tier in a conditional tree freezes the evaluated branch. The
	// branch's amount must have no other dependency on a missing input.
	price := selected.Arguments[1]
	if len(leaves) < 2 || ast.Find(price, historicalBillingProbe) != nil {
		return "", false
	}
	switch price.(type) {
	case *ast.IntegerNode, *ast.FloatNode:
		return selected.String(), true
	default:
		return selected.String(), false
	}
}

func collectHistoricalStatementTiers(node ast.Node, leaves *[]*ast.CallNode) bool {
	switch node := node.(type) {
	case *ast.ConditionalNode:
		return collectHistoricalStatementTiers(node.Exp1, leaves) && collectHistoricalStatementTiers(node.Exp2, leaves)
	case *ast.CallNode:
		if !historicalStatementCall(node, "tier", 2) {
			return false
		}
		if label, ok := node.Arguments[0].(*ast.StringNode); !ok || label.Value == "" {
			return false
		}
		*leaves = append(*leaves, node)
		return true
	default:
		return false
	}
}

func historicalStatementCall(call *ast.CallNode, name string, arguments int) bool {
	id, ok := call.Callee.(*ast.IdentifierNode)
	return ok && id.Value == name && len(call.Arguments) == arguments
}

func historicalStatementImageTokens(facts customerExportFacts, semantic string) (int64, bool) {
	raw := facts.other["image_output"]
	marker := facts.other["image"]
	if len(raw) != 0 || len(marker) != 0 {
		tokens, ok := billingBreakdownNonNegativeInt(raw)
		image, marked := billingReconciliationBool(marker)
		return tokens, ok && marked && image && tokens > 0
	}
	// The historical synchronous text and OpenAI image writer used the
	// image_output key for input image tokens and emitted it only above zero.
	path := facts.string("request_path")
	return 0, path != "" && (semantic == "anthropic" || semantic == "openai" && strings.HasPrefix(path, "/v1/images/"))
}

func frozenTieredAmountsWithToolFees(facts customerExportFacts, parsed parsedBillingReconciliationLog, modelBeforeGroup float64) (int64, int64, string) {
	for _, legacy := range []string{"fee_quota", "image_generation_call_price", "web_search_price", "file_search_price"} {
		if len(facts.other[legacy]) != 0 {
			return 0, 0, BillingOriginalReplayUnverified
		}
	}
	var tools []struct {
		Count int      `json:"count"`
		Price *float64 `json:"price"`
	}
	if common.Unmarshal(facts.other["tool_surcharges"], &tools) != nil || len(tools) == 0 {
		return 0, 0, BillingOriginalReplayUnverified
	}
	group := decimal.NewFromFloat(*parsed.discountRatio)
	contract, ok := historicalStatementContractRatio(parsed)
	if !ok || group.IsNegative() {
		return 0, 0, BillingOriginalReplayMissingContext
	}
	var surchargeOriginal, surchargeBooked decimal.Decimal
	for _, tool := range tools {
		if tool.Count <= 0 || tool.Count > relaycommon.MaxBillableToolCallCount || tool.Price == nil || *tool.Price < 0 {
			return 0, 0, BillingOriginalReplayUnverified
		}
		amount := decimal.NewFromFloat(*tool.Price).Mul(decimal.NewFromInt(int64(tool.Count))).Div(decimal.NewFromInt(1000))
		surchargeOriginal = surchargeOriginal.Add(amount.Mul(decimal.NewFromInt(historicalCustomerQuotaPerUnit)))
		surchargeBooked = surchargeBooked.Add(amount.Mul(group).Mul(decimal.NewFromInt(historicalCustomerQuotaPerUnit)))
	}
	model := decimal.NewFromFloat(modelBeforeGroup)
	net, err := common.QuotaFromDecimalStrict(model.Mul(group).Mul(contract).Add(surchargeBooked))
	original, originalErr := common.QuotaFromDecimalStrict(model.Add(surchargeOriginal))
	if err != nil || originalErr != nil {
		return 0, 0, BillingOriginalReplayMismatch
	}
	return int64(original), int64(net), ""
}

func historicalStatementContractRatio(parsed parsedBillingReconciliationLog) (decimal.Decimal, bool) {
	switch billingContractApplicableState(parsed) {
	case combinationContractApplicableNo:
		return decimal.NewFromInt(1), parsed.contractDiscountRatio == nil
	case combinationContractApplicableYes:
		if ratio := parsed.contractDiscountRatio; ratio != nil && *ratio > 0 && *ratio <= 1 {
			return decimal.NewFromFloat(*ratio), true
		}
	}
	return decimal.Zero, false
}
