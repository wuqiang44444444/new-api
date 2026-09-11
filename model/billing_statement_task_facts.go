package model

import (
	"encoding/base64"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
)

// BillingStatementExpressionMode classifies the frozen expression, never the
// model name or today's pricing. Other usage units need their own display;
// unknown is preferable to falsely calling seconds or credits token billing.
func BillingStatementExpressionMode(expression string, usageUnits map[string]string) string {
	vars := billingexpr.UsedVars(expression)
	if vars == nil {
		return BillingReconciliationModeUnknown
	}
	if vars["u"] {
		program, err := billingexpr.CompileFromCache(expression)
		if err != nil {
			return BillingReconciliationModeUnknown
		}
		valid, token := true, false
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
			if !ok || callee.Value != "u" {
				return false
			}
			calls++
			if len(call.Arguments) != 1 {
				valid = false
				return false
			}
			key, ok := call.Arguments[0].(*ast.StringNode)
			if !ok {
				valid = false
				return false
			}
			switch usageUnits[key.Value] {
			case "token":
				token = true
			case "enum", "boolean": // Declared conditions do not change the meter unit.
			default:
				valid = false
			}
			return false
		})
		if valid && token && calls == identifiers {
			return BillingReconciliationModeToken
		}
		return BillingReconciliationModeUnknown
	}
	for _, name := range []string{"p", "c", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao", "len"} {
		if vars[name] {
			return BillingReconciliationModeToken
		}
	}
	return BillingReconciliationModeUnknown
}

func billingStatementTaskFacts(log billingReconciliationLog, other, snapshot map[string]json.RawMessage, parsed *parsedBillingReconciliationLog) {
	isTask, _ := billingReconciliationBool(other["is_task"])
	taskID := billingBreakdownString(other["task_id"])
	event := billingBreakdownString(other["task_billing_event"])
	_, hasActual := other["actual_quota"]
	_, hasPreconsume := other["pre_consumed_quota"]
	adjustment := hasActual || hasPreconsume || event == "adjustment"
	parsed.isRequest = log.Type == LogTypeConsume && !adjustment && (event == "create" || isTask || taskID == "")
	parsed.isRefund = log.Type == LogTypeRefund && !adjustment

	encoded := billingBreakdownString(billingReconciliationSnapshotRaw(snapshot, other, "expr_b64"))
	if encoded != "" {
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
		parsed.billingMode = BillingStatementExpressionMode(string(expression), units)
		return
	}
	// Version 1 task snapshots forced per_call even for a zero model_price and
	// measured output tokens. These measured facts disprove that classification.
	price, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "model_price"))
	if (isTask || taskID != "") && ok && price == 0 && log.CompletionTokens > 0 {
		parsed.billingMode = BillingReconciliationModeToken
	}
}
