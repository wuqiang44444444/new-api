package model

import (
	"encoding/base64"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
)

const customerUsagePerImage = "per_image"

func customerUsageLogMeter(row usageLogRow) (mode string, imageCount int64, imageKnown bool) {
	if row.fact.Type != LogTypeConsume || row.parsed.fundingAdjustmentFailed || row.parsed.unavailable {
		return BillingReconciliationModeUnknown, 0, false
	}
	facts := loadCustomerExportFacts(row.fact.Other)
	unit := facts.string("billing_unit")
	if unit == string(billingexpr.BillingUnitRequest) {
		raw, err := base64.StdEncoding.DecodeString(facts.string("expr_b64"))
		if err != nil {
			return BillingReconciliationModeUnknown, 0, false
		}
		mode = customerUsageFixedTierMeter(string(raw), facts.string("matched_tier"))
		if mode == customerUsagePerImage {
			imageCount, imageKnown = billingBreakdownNonNegativeInt(facts.raw("image_count"))
			imageKnown = imageKnown && imageCount > 0 && imageCount <= int64(dto.MaxImageN)
		}
		return mode, imageCount, imageKnown
	}
	// An expression with typed task meters can also emit the engine's token
	// default. Give the frozen settlement meter precedence in that case.
	switch row.parsed.billingMode {
	case BillingReconciliationModePerSecond, BillingReconciliationModeDurationAndCount:
		return row.parsed.billingMode, 0, false
	}
	if unit == string(billingexpr.BillingUnitToken) {
		return BillingReconciliationModeToken, 0, false
	}
	if unit != "" {
		return BillingReconciliationModeUnknown, 0, false
	}
	if encoded := facts.string("expr_b64"); encoded != "" {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || billingexpr.UsesFixedPricing(string(raw)) {
			return BillingReconciliationModeUnknown, 0, false
		}
		var units map[string]string
		if rawUnits := facts.raw("usage_units"); len(rawUnits) > 0 && common.Unmarshal(rawUnits, &units) != nil {
			return BillingReconciliationModeUnknown, 0, false
		}
		return BillingStatementExpressionMode(string(raw), units), 0, false
	}
	snapshotMode := billingBreakdownString(facts.snapshot["billing_mode"])
	price, priced := facts.float("model_price")
	if priced && price > 0 && (snapshotMode == "" || snapshotMode == BillingReconciliationModePerCall) {
		// The native fixed-price Images path multiplies its model price by n.
		// Its recorded completion count replaces that multiplier when observed.
		switch facts.string("request_path") {
		case "/v1/images/generations", "/v1/images/edits":
			count, known := billingBreakdownNonNegativeInt(facts.raw("completed_image_count"))
			return customerUsagePerImage, count, known && count > 0 && count <= int64(dto.MaxImageN)
		}
	}
	if ValidBillingReconciliationMode(snapshotMode) {
		return snapshotMode, 0, false
	}
	if priced && price > 0 {
		return BillingReconciliationModePerCall, 0, false
	}
	if ratio, ok := facts.float("model_ratio"); ok && ratio > 0 {
		return BillingReconciliationModeToken, 0, false
	}
	return BillingReconciliationModeUnknown, 0, false
}

// A matched tier identifies the executed fixed leaf. A reused tier name is
// sufficient only if every matching fixed leaf uses the same meter.
const (
	usageFixedCall uint8 = 1 << iota
	usageFixedImage
	usageTokenTier
)

func customerUsageFixedTierMeter(expression, tier string) string {
	if tier == "" || !billingexpr.UsesFixedPricing(expression) {
		return BillingReconciliationModeUnknown
	}
	program, err := billingexpr.CompileFromCache(expression)
	if err != nil {
		return BillingReconciliationModeUnknown
	}
	meters := customerUsageTierMeters(program.Node(), tier, false) &^ usageTokenTier
	switch meters {
	case usageFixedCall:
		return BillingReconciliationModePerCall
	case usageFixedImage:
		return customerUsagePerImage
	default:
		return BillingReconciliationModeUnknown
	}
}

func customerUsageTierMeters(node ast.Node, tier string, perImage bool) uint8 {
	switch n := node.(type) {
	case *ast.ConditionalNode:
		return customerUsageTierMeters(n.Exp1, tier, perImage) | customerUsageTierMeters(n.Exp2, tier, perImage)
	case *ast.BinaryNode:
		if n.Operator == "*" {
			left, _ := n.Left.(*ast.IdentifierNode)
			right, _ := n.Right.(*ast.IdentifierNode)
			return customerUsageTierMeters(n.Left, tier, perImage || (right != nil && right.Value == "image_count")) |
				customerUsageTierMeters(n.Right, tier, perImage || (left != nil && left.Value == "image_count"))
		}
	case *ast.CallNode:
		if historicalStatementCall(n, "tier", 2) {
			label, _ := n.Arguments[0].(*ast.StringNode)
			price, _ := n.Arguments[1].(*ast.CallNode)
			if label == nil || label.Value != tier {
				return 0
			}
			if price == nil || !historicalStatementCall(price, "fixed", 1) {
				return usageTokenTier
			}
			if perImage {
				return usageFixedImage
			}
			return usageFixedCall
		}
	}
	return 0
}

func customerUsageTaskMeter(task *usageTaskRecord, pieces *usageTaskPieces, settled bool) (string, int64, bool) {
	if !settled || task.Status != TaskStatusSuccess {
		return BillingReconciliationModeUnknown, 0, false
	}
	bc := task.PrivateData.BillingContext
	var snapshot *billingexpr.BillingSnapshot
	var calculation *billingexpr.Calculation
	if bc != nil {
		snapshot = bc.TieredSnapshot
		calculation = bc.SettlementCalculation
	}
	if async := task.PrivateData.AsyncBilling; async != nil && async.TieredSnapshot != nil {
		snapshot = async.TieredSnapshot
		if async.CalculationSource == "settlement" && async.Calculation != nil {
			calculation = async.Calculation
		}
	}
	image := task.PrivateData.ImageTask
	if snapshot != nil {
		if billingexpr.UsesFixedPricing(snapshot.ExprString) {
			program, err := billingexpr.CompileFromCache(snapshot.ExprString)
			if calculation == nil || err != nil {
				return BillingReconciliationModeUnknown, 0, false
			}
			switch customerUsageTierMeters(program.Node(), calculation.MatchedTier, false) {
			case usageTokenTier:
				return BillingReconciliationModeToken, 0, false
			case usageFixedCall:
				return BillingReconciliationModePerCall, 0, false
			case usageFixedImage:
				// The native task's charged quantity is in its protected billing
				// probe. The delivery manifest is an independent fact.
				return customerUsagePerImage, 0, false
			default:
				return BillingReconciliationModeUnknown, 0, false
			}
		}
		if image != nil && snapshot.UsageUnits["image_count"] == "count" && len(snapshot.UsageUnits) == 1 {
			vars := billingexpr.UsedVars(snapshot.ExprString)
			for _, name := range []string{"p", "c", "len", "cr", "cc", "cc1h", "img", "img_cr", "img_o", "ai", "ao"} {
				if vars[name] {
					return BillingReconciliationModeUnknown, 0, false
				}
			}
			program, err := billingexpr.CompileFromCache(snapshot.ExprString)
			if err == nil && ast.Find(program.Node(), func(node ast.Node) bool {
				call, ok := node.(*ast.CallNode)
				if !ok || !historicalStatementCall(call, "u", 1) {
					return false
				}
				name, ok := call.Arguments[0].(*ast.StringNode)
				return ok && name.Value == "image_count"
			}) != nil {
				if calculation != nil {
					if raw, err := common.Marshal(calculation.UsageFacts["image_count"]); err == nil {
						count, known := billingBreakdownNonNegativeInt(raw)
						return customerUsagePerImage, count, known && count > 0 && count <= int64(dto.MaxImageN)
					}
				}
				return customerUsagePerImage, 0, false
			}
		}
		return BillingStatementExpressionMode(snapshot.ExprString, snapshot.UsageUnits), 0, false
	}
	if image != nil && bc != nil && image.NativeRequest != nil {
		if bc.PerCallBilling {
			count := int64(image.ImageCount)
			return customerUsagePerImage, count, count > 0 && count <= int64(dto.MaxImageN)
		}
		return BillingReconciliationModeToken, 0, false
	}
	if bc != nil && bc.PerCallBilling {
		return BillingReconciliationModePerCall, 0, false
	}
	if pieces.secondsKnown || pieces.billingMode == BillingReconciliationModePerSecond {
		return BillingReconciliationModePerSecond, 0, false
	}
	return BillingReconciliationModeUnknown, 0, false
}

func setCustomerUsageMeter(metrics *UsageAnalyticsMetrics, mode string, imageCount int64, imageKnown bool) {
	metrics.ImageCount = 0
	metrics.ImageCountKnown = false
	metrics.ImageCountMissingRows = 0
	if mode != customerUsagePerImage {
		return
	}
	metrics.InputTokens, metrics.OutputTokens = 0, 0
	metrics.CacheReadTokens, metrics.CacheWriteTokens = 0, 0
	metrics.RowsMissingTokens = 0
	metrics.TokenDetails = nil
	metrics.ImageCountKnown = imageKnown
	if imageKnown {
		metrics.ImageCount = imageCount
	} else {
		metrics.ImageCountMissingRows = 1
	}
}
