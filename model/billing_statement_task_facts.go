package model

import (
	"encoding/base64"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
)

// BillingStatementExpressionMode classifies only frozen meter facts. The
// reserved _task.duration_seconds probe has always been seconds; arbitrary
// client parameter names and undeclared u() units are not meter evidence.
func BillingStatementExpressionMode(expression string, usageUnits map[string]string) string {
	vars := billingexpr.UsedVars(expression)
	if vars == nil {
		return BillingReconciliationModeUnknown
	}
	token, seconds, count := false, false, false
	for _, name := range []string{"p", "c", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao", "len"} {
		token = token || vars[name]
	}
	program, err := billingexpr.CompileFromCache(expression)
	if err != nil {
		return BillingReconciliationModeUnknown
	}
	valid, untypedParam := true, false
	identifiers, calls := 0, 0
	ast.Find(program.Node(), func(node ast.Node) bool {
		if id, ok := node.(*ast.IdentifierNode); ok && id.Value == "u" {
			identifiers++
		}
		call, ok := node.(*ast.CallNode)
		if !ok {
			return false
		}
		callee, ok := call.Callee.(*ast.IdentifierNode)
		if !ok || (callee.Value != "u" && callee.Value != "param") {
			return false
		}
		if callee.Value == "u" {
			calls++
		}
		if len(call.Arguments) != 1 {
			valid = false
			return false
		}
		key, ok := call.Arguments[0].(*ast.StringNode)
		if !ok {
			if callee.Value == "u" {
				valid = false
			} else {
				untypedParam = true
			}
			return false
		}
		if callee.Value == "param" {
			switch key.Value {
			case "_task.duration_seconds":
				seconds = true
			case "_task.resolution", "_task.has_video_input", "_task.generate_audio", "_task.input_mode", "_task.control_mode", "_task.size_multiplier":
				// Historical probe conditions and the dimensionless size multiplier
				// do not introduce another meter. This only classifies frozen facts;
				// it does not authorize these fields in new pricing contracts.
			default:
				untypedParam = true
			}
			return false
		}
		switch usageUnits[key.Value] {
		case "token":
			token = true
		case "second":
			seconds = true
		case "count":
			count = true
		case "enum", "boolean":
		default:
			valid = false
		}
		return false
	})
	if !valid || calls != identifiers || (token && seconds) {
		return BillingReconciliationModeUnknown
	}
	if count {
		if seconds && !untypedParam {
			return BillingReconciliationModeDurationAndCount
		}
		return BillingReconciliationModeUnknown
	}
	if seconds && !untypedParam {
		return BillingReconciliationModePerSecond
	}
	if token {
		return BillingReconciliationModeToken
	}
	return BillingReconciliationModeUnknown
}

func billingStatementTaskFacts(log billingReconciliationLog, other, snapshot map[string]json.RawMessage, parsed *parsedBillingReconciliationLog) {
	isTask, _ := billingReconciliationBool(other["is_task"])
	taskID := billingBreakdownString(other["task_id"])
	event := billingBreakdownString(other["task_billing_event"])
	_, hasActual := other["actual_quota"]
	_, hasPreconsume := other["pre_consumed_quota"]
	adjustment := isTaskBillingAdjustment(event, hasActual, hasPreconsume)
	parsed.isRequest = log.Type == LogTypeConsume && !adjustment && (event == "create" || isTask || taskID == "")
	parsed.isRefund = log.Type == LogTypeRefund && !adjustment

	encoded := billingBreakdownString(billingReconciliationSnapshotRaw(snapshot, other, "expr_b64"))
	if encoded != "" {
		parsed.hasExpression = true
		expression, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			parsed.billingMode = BillingReconciliationModeUnknown
			parsed.unavailable = true
			return
		}
		var units map[string]string
		if raw := billingReconciliationSnapshotRaw(snapshot, other, "usage_units"); len(raw) > 0 {
			if common.Unmarshal(raw, &units) != nil {
				parsed.unavailable = true
				units = nil
			}
		}
		unit := billingBreakdownString(billingReconciliationSnapshotRaw(snapshot, other, "billing_unit"))
		parsed.billingMode = BillingStatementSettledExpressionMode(string(expression), units, unit)
		return
	}
	if isTask && event == "create" {
		switch billingBreakdownString(other["billing_unit"]) {
		case string(billingexpr.BillingUnitToken):
			parsed.billingMode = BillingReconciliationModeToken
			return
		case string(billingexpr.BillingUnitRequest):
			parsed.billingMode = BillingReconciliationModePerCall
			return
		}
	}
	// Version 1 task snapshots forced per_call even with a zero fixed price.
	// A completed async image can also carry -1 for an unconfigured fixed price.
	price, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "model_price"))
	legacyImageTokenPrice := price == -1 && parsed.hasImageCount && event == "create" && !parsed.hasAuxiliaryCharge
	if (isTask || taskID != "") && ok && (legacyImageTokenPrice || (price == 0 && log.Quota > 0)) {
		measuredTokens := log.CompletionTokens > 0 || (legacyImageTokenPrice && log.PromptTokens > 0)
		if measuredTokens && parsed.billingMode != BillingReconciliationModePerSecond && parsed.billingMode != BillingReconciliationModeDurationAndCount {
			parsed.billingMode = BillingReconciliationModeToken
		} else if parsed.billingMode == BillingReconciliationModePerCall {
			parsed.billingMode = BillingReconciliationModeUnknown
		}
		if log.Type == LogTypeRefund && parsed.billingMode == BillingReconciliationModeUnknown {
			parsed.refundTaskID = taskID
		}
	}
}

// Customer refunds are not provider credits. Only a task settlement adjustment
// may carry final usage on a refund row; ordinary refunds never enter this view.
func isProviderTaskUsageAdjustment(log billingReconciliationLog) bool {
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil || billingBreakdownString(other["task_id"]) == "" {
		return false
	}
	event := billingBreakdownString(other["task_billing_event"])
	_, actual := other["actual_quota"]
	_, pre := other["pre_consumed_quota"]
	return isTaskBillingAdjustment(event, actual, pre)
}

func isTaskBillingAdjustment(event string, hasActual, hasPreconsume bool) bool {
	return event == "adjustment" || (event == "" && hasActual && hasPreconsume)
}
