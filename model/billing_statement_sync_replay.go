package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
)

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

func historicalStatementTierExpression(expression, tier string, unit billingexpr.BillingUnit, facts customerExportFacts) (string, bool) {
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return "", false
	}
	root := tree.Node
	imageFactor := false
	if unit == billingexpr.BillingUnitRequest {
		if product, ok := root.(*ast.BinaryNode); ok && product.Operator == "*" {
			if count, ok := product.Right.(*ast.IdentifierNode); ok && count.Value == "image_count" {
				root, imageFactor = product.Left, true
			}
		}
	}
	var leaves []*ast.CallNode
	if !collectHistoricalStatementTiers(root, &leaves) {
		return "", false
	}
	var selected *ast.CallNode
	for _, leaf := range leaves {
		if unit == billingexpr.BillingUnitRequest {
			fixed, ok := leaf.Arguments[1].(*ast.CallNode)
			if !ok || !historicalStatementCall(fixed, "fixed", 1) {
				return "", false
			}
		}
		if leaf.Arguments[0].(*ast.StringNode).Value == tier {
			if selected != nil && unit != billingexpr.BillingUnitRequest {
				return "", false
			}
			selected = leaf
		}
	}
	if selected == nil {
		return "", false
	}
	if unit == billingexpr.BillingUnitRequest {
		price, ok := facts.float("fixed_price")
		if !ok || price < 0 {
			return "", false
		}
		// The writer saved the evaluated fixed() operand before image count.
		value := fmt.Sprintf("tier(%q, fixed(%s))", tier, strconv.FormatFloat(price, 'g', -1, 64))
		if imageFactor {
			value += " * image_count"
		}
		return value, false
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

func replayTieredWithToolFees(log billingReconciliationLog, facts customerExportFacts, parsed parsedBillingReconciliationLog, modelBeforeGroup float64) (int64, string) {
	for _, legacy := range []string{"fee_quota", "image_generation_call_price", "web_search_price", "file_search_price"} {
		if len(facts.other[legacy]) != 0 {
			return 0, BillingOriginalReplayUnverified
		}
	}
	var tools []struct {
		Count int      `json:"count"`
		Price *float64 `json:"price"`
	}
	if common.Unmarshal(facts.other["tool_surcharges"], &tools) != nil || len(tools) == 0 {
		return 0, BillingOriginalReplayUnverified
	}
	group := decimal.NewFromFloat(*parsed.discountRatio)
	contract, ok := historicalStatementContractRatio(parsed)
	if !ok || group.IsNegative() {
		return 0, BillingOriginalReplayMissingContext
	}
	var surchargeOriginal, surchargeBooked decimal.Decimal
	for _, tool := range tools {
		if tool.Count <= 0 || tool.Count > relaycommon.MaxBillableToolCallCount || tool.Price == nil || *tool.Price < 0 {
			return 0, BillingOriginalReplayUnverified
		}
		amount := decimal.NewFromFloat(*tool.Price).Mul(decimal.NewFromInt(int64(tool.Count))).Div(decimal.NewFromInt(1000))
		surchargeOriginal = surchargeOriginal.Add(amount.Mul(decimal.NewFromInt(historicalCustomerQuotaPerUnit)))
		surchargeBooked = surchargeBooked.Add(amount.Mul(group).Mul(decimal.NewFromInt(historicalCustomerQuotaPerUnit)))
	}
	model := decimal.NewFromFloat(modelBeforeGroup)
	net, err := common.QuotaFromDecimalStrict(model.Mul(group).Mul(contract).Add(surchargeBooked))
	original, originalErr := common.QuotaFromDecimalStrict(model.Add(surchargeOriginal))
	if err != nil || originalErr != nil || net != log.Quota {
		return 0, BillingOriginalReplayMismatch
	}
	return int64(original), ""
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
