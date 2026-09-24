// Package seedancebilling owns the single Seedance Link billing field
// contract: the u() usage fields a Seedance expression may reference, their
// host-owned types and units, and which fields carry a trusted measured
// customer meter at settlement. Expression save validation, pre-consume fact
// construction, settlement, usage recovery, database funding protection and
// the inventory tool all read this one description instead of maintaining
// per-call-site field tables.
package seedancebilling

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/expr-lang/expr/ast"
)

// TokenUsageKey is the only measured customer meter: trusted provider
// completion tokens. At submission it carries the administrator budget; at
// settlement it may only be replaced by accepted actual usage. Every other
// declared field is a frozen request condition.
const TokenUsageKey = "tokens"

// Common usage fields derived by the host billing probe for every published
// Seedance video protocol. Enum and bound values mirror the probe contract in
// relay/channel/task/seedance/billing_probe.go.
func commonUsageFields() map[string]jsplugin.UsageFieldSchema {
	return map[string]jsplugin.UsageFieldSchema{
		TokenUsageKey:      {Type: "number", Unit: "token"},
		"resolution":       {Enum: []string{"480p", "720p", "1080p", "4k"}},
		"has_video_input":  {Type: "boolean"},
		"duration_seconds": {Type: "number", Unit: "second"},
		"generate_audio":   {Type: "boolean"},
		"input_mode":       {Enum: []string{"text", "single_image", "multi_image", "multi_modal"}},
		"control_mode":     {Enum: []string{"none", "reference", "end_frame"}},
	}
}

// protocolExtraFields returns fields that exist only for the selected
// protocol. The values mirror the fixed protocol configuration used by the
// billing probe and its smoke requests; a protocol-only field is never
// advertised to other Seedance channels.
func protocolExtraFields(protocol dto.VideoUpstreamProtocol) map[string]jsplugin.UsageFieldSchema {
	switch protocol {
	case dto.VideoUpstreamProtocolFeicaiVideosV1:
		return map[string]jsplugin.UsageFieldSchema{
			"ratio":        {Enum: feicaiRatioEnum()},
			"billing_mode": {Enum: []string{feicai.BillingModePerSecond}},
		}
	case dto.VideoUpstreamProtocolFunCloudModelArkV3:
		return map[string]jsplugin.UsageFieldSchema{
			"billing_mode": {Enum: []string{"per-second", "per-token"}},
		}
	default:
		return nil
	}
}

// feicaiRatioEnum derives the declared ratio values from the code-registered
// Feicai model specs, so the field contract cannot drift from the protocol
// configuration. The legacy size_multiplier probe field is deliberately not
// declared: it is a migration-compat field that always evaluates to 1 and new
// contracts must not price on it.
func feicaiRatioEnum() []string {
	values := make(map[string]struct{})
	for _, spec := range feicai.CurrentModelSpecs() {
		for _, ratio := range spec.Ratios {
			values[ratio] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(values))
	for value := range values {
		ordered = append(ordered, value)
	}
	sort.Strings(ordered)
	return ordered
}

// UsageFieldsForProtocol returns the declared u() field contract for one
// protocol: the common host probe fields plus that protocol's declared
// extras. The returned map is a fresh copy shared by the editor, save
// validation, pre-consume and settlement.
func UsageFieldsForProtocol(protocol dto.VideoUpstreamProtocol) map[string]jsplugin.UsageFieldSchema {
	fields := commonUsageFields()
	// CMCC's registered adapter accepts only these resolutions. Do not ask
	// administrators to invent a 4k price for a request it cannot submit.
	if protocol == dto.VideoUpstreamProtocolModelArkV3CMCC {
		fields["resolution"] = jsplugin.UsageFieldSchema{Enum: []string{"480p", "720p", "1080p"}}
	}
	for name, field := range protocolExtraFields(protocol) {
		fields[name] = field
	}
	return fields
}

// IntersectUsageFields narrows schemas to the fields declared by every
// attributed protocol, so a model served by channels with different protocol
// extras is never shown a field its runtime cannot supply.
func IntersectUsageFields(schemas ...map[string]jsplugin.UsageFieldSchema) map[string]jsplugin.UsageFieldSchema {
	if len(schemas) == 0 {
		return map[string]jsplugin.UsageFieldSchema{}
	}
	result := schemas[0]
	for _, schema := range schemas[1:] {
		result = intersectSchemas(result, schema)
	}
	return result
}

func intersectSchemas(left, right map[string]jsplugin.UsageFieldSchema) map[string]jsplugin.UsageFieldSchema {
	result := make(map[string]jsplugin.UsageFieldSchema, len(left))
	for name, field := range left {
		other, ok := right[name]
		if !ok {
			continue
		}
		if len(field.Enum) > 0 && len(other.Enum) > 0 {
			shared := make([]string, 0, len(field.Enum))
			for _, value := range field.Enum {
				if slices.Contains(other.Enum, value) {
					shared = append(shared, value)
				}
			}
			if len(shared) == 0 {
				continue
			}
			field.Enum = shared
		}
		result[name] = field
	}
	return result
}

// UsageUnitsForSchema projects declared meter units for the frozen
// BillingSnapshot.UsageUnits display metadata. It never affects evaluation.
func UsageUnitsForSchema(schema map[string]jsplugin.UsageFieldSchema) map[string]string {
	units := make(map[string]string, len(schema))
	for name, field := range schema {
		switch {
		case len(field.Enum) > 0:
			units[name] = "enum"
		case field.Type == "boolean":
			units[name] = "boolean"
		default:
			units[name] = field.Unit
		}
	}
	return units
}

// RequiresMeasuredTokens reports whether the expression reads the measured
// token meter, the only dependency that must wait for trusted provider usage
// at settlement. Dependencies on frozen request conditions (resolution,
// duration, input mode) never require waiting. A u() call whose key cannot be
// proven static fails closed as measured.
func RequiresMeasuredTokens(expression string) bool {
	keys := staticUsageKeys(expression)
	if keys == nil {
		return true
	}
	return keys[TokenUsageKey]
}

// RequiresTokenBudget is the protocol policy shared by configuration, runtime
// and offline checks. Frozen-only FunCloud/Synlink prices need no token budget.
func RequiresTokenBudget(protocol dto.VideoUpstreamProtocol, expression string) bool {
	return (protocol != dto.VideoUpstreamProtocolFunCloudModelArkV3 && protocol != dto.VideoUpstreamProtocolSynlinkVideoV1) || RequiresMeasuredTokens(expression)
}

// RequiresMeasuredTaskUsage dispatches on the frozen snapshot's unit
// contract: u() expressions wait only for measured fields; legacy c/_task
// expressions keep the generic usage dependency.
func RequiresMeasuredTaskUsage(snapshot *billingexpr.BillingSnapshot) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.TaskUsageBilling {
		return RequiresMeasuredTokens(snapshot.ExprString)
	}
	return billingexpr.RequiresUsage(snapshot.ExprString)
}

// ControlledFacts projects the frozen billing probe body and the customer
// token meter into u() evaluation facts. At submission the meter is the
// administrator budget; at settlement it is the accepted actual usage, or a
// value that the measured-dependency rule prevents from being charged. The
// result is an evaluation-only projection: it never replaces the frozen probe
// or accepted usage as the stored authority.
func ControlledFacts(probeBody []byte, tokens int) (map[string]any, error) {
	var wrapper struct {
		Task map[string]any `json:"_task"`
	}
	if len(probeBody) > 0 {
		if err := common.Unmarshal(probeBody, &wrapper); err != nil {
			return nil, fmt.Errorf("decode frozen task billing probe: %w", err)
		}
	}
	facts := make(map[string]any, len(wrapper.Task)+1)
	for key, value := range wrapper.Task {
		facts[key] = value
	}
	// 与探针投影的 JSON 数值类型一致（float64），保证预扣与结算求值输入同型。
	facts[TokenUsageKey] = float64(tokens)
	return facts, nil
}

// tokenExpressionVars are the plain-text token variables. A Seedance task
// contract prices through declared u() fields only; mixing c/_task semantics
// with u() amounts would put two unit systems into one expression.
var tokenExpressionVars = []string{"p", "c", "len", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao"}

// ValidateTaskExpression enforces the Seedance u() save contract: the
// expression compiles, prices only through declared u() fields, wraps every
// price branch in tier(), stays deterministic across pre-consume and
// settlement, and evaluates non-negative over schema-driven vectors.
func ValidateTaskExpression(expression string, schema map[string]jsplugin.UsageFieldSchema) error {
	if err := ValidateTaskExpressionInputs(expression, schema); err != nil {
		return err
	}
	return smokeTestTaskExpression(expression, schema)
}

// ValidateTaskExpressionInputs validates the static input contract without
// evaluating smoke vectors on the submission path.
func ValidateTaskExpressionInputs(expression string, schema map[string]jsplugin.UsageFieldSchema) error {
	_, err := billingexpr.CompileFromCache(expression)
	if err != nil {
		return err
	}
	vars := billingexpr.UsedVars(expression)
	if err := billingexpr.ValidateAsyncDeterminism(expression); err != nil {
		return err
	}
	for _, name := range tokenExpressionVars {
		if vars[name] {
			return fmt.Errorf("task expression must price through declared u() fields; %q belongs to the token expression contract", name)
		}
	}
	if vars["param"] {
		return fmt.Errorf("task expression must read frozen conditions through declared u() fields, not param()")
	}
	if err := billingexpr.UnknownIdentifier(expression); err != nil {
		return err
	}
	_, err = ReferencedUsageFields(expression, schema)
	return err
}

// ReferencedUsageFields returns only the declared fields the expression reads.
// Validation and offline proof use the same exact-key dependency analysis, so
// unused protocol dimensions cannot crowd actual price conditions out.
func ReferencedUsageFields(expression string, schema map[string]jsplugin.UsageFieldSchema) (map[string]jsplugin.UsageFieldSchema, error) {
	keys := staticUsageKeys(expression)
	if keys == nil {
		return nil, fmt.Errorf("task expression must use direct u() calls with exact literal field names")
	}
	fields := make(map[string]jsplugin.UsageFieldSchema, len(keys))
	for key := range keys {
		field, declared := schema[key]
		if !declared {
			return nil, fmt.Errorf("usage key %q is not declared by the Seedance field contract", key)
		}
		fields[key] = field
	}
	return fields, nil
}

// staticUsageKeys returns nil for any unproven reference, including function
// aliases mixed with otherwise valid direct calls. Never normalize key spelling:
// the engine looks up the exact string. An empty non-nil set has no dependency.
func staticUsageKeys(expression string) map[string]bool {
	program, err := billingexpr.CompileFromCache(expression)
	if err != nil {
		return nil
	}
	keys := map[string]bool{}
	identifiers, calls := 0, 0
	invalid := false
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
			invalid = true
			return false
		}
		key, ok := call.Arguments[0].(*ast.StringNode)
		if !ok || key.Value == "" || strings.TrimSpace(key.Value) != key.Value {
			invalid = true
			return false
		}
		keys[key.Value] = true
		return false
	})
	if invalid || calls != identifiers {
		return nil
	}
	return keys
}

// smokeTestTaskExpression exercises every declared field at its boundary
// values of referenced fields and requires every evaluated branch to
// be finite, non-negative and tier-wrapped. Link enumerates all referenced
// boundaries; native plugins retain their upstream sampling policy. Both
// use the same engine, without changing setting/billing_setting.
func smokeTestTaskExpression(expression string, schema map[string]jsplugin.UsageFieldSchema) error {
	// Only referenced fields affect the result. Enumerate their complete declared
	// boundaries so irrelevant fields cannot crowd real price branches out.
	fields, err := ReferencedUsageFields(expression, schema)
	if err != nil {
		return err
	}
	vectors, err := taskUsageVectors(fields)
	if err != nil {
		return err
	}
	var rate *billingexpr.ExchangeRateContext
	if billingexpr.UsesExchangeRate(expression) {
		rate, err = operation_setting.CurrentUsdExchangeRateContext()
		if err != nil {
			return fmt.Errorf("expression requires a valid USDExchangeRate setting: %w", err)
		}
	}
	for _, usage := range vectors {
		result, trace, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: usage, ExchangeRate: rate})
		if err != nil {
			return fmt.Errorf("usage vector %v: run failed: %w", usage, err)
		}
		if trace.MatchedTier == "" {
			return fmt.Errorf("billing expression must wrap every price branch with tier(name, value)")
		}
		if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
			return fmt.Errorf("usage vector %v: result must be finite and non-negative, got %f", usage, result)
		}
	}
	return nil
}

const maxTaskUsageVectors = 4096

type usageDimension struct {
	name   string
	values []any
}

// taskUsageVectors mirrors the plugin schema vector generation: numeric
// fields at 0/1/host ceiling, booleans at false/true, enums over their
// declared values. Excessive combinations fail validation without truncation.
func taskUsageVectors(schema map[string]jsplugin.UsageFieldSchema) ([]map[string]any, error) {
	names := make([]string, 0, len(schema))
	for name := range schema {
		names = append(names, name)
	}
	sort.Strings(names)

	dimensions := make([]usageDimension, 0, len(names))
	for _, name := range names {
		field := schema[name]
		if len(field.Enum) > 0 {
			values := make([]any, len(field.Enum))
			for index, value := range field.Enum {
				values[index] = value
			}
			dimensions = append(dimensions, usageDimension{name: name, values: values})
			continue
		}
		if field.Type == "boolean" {
			dimensions = append(dimensions, usageDimension{name: name, values: []any{false, true}})
			continue
		}
		limit := float64(common.MaxQuota)
		if field.Unit == "second" {
			limit = 3600
		}
		dimensions = append(dimensions, usageDimension{
			name:   name,
			values: []any{float64(0), float64(1), limit},
		})
	}

	count := 1
	for _, dimension := range dimensions {
		if count > maxTaskUsageVectors/len(dimension.values) {
			return nil, fmt.Errorf("complete usage validation exceeds %d combinations", maxTaskUsageVectors)
		}
		count *= len(dimension.values)
	}
	vectors := []map[string]any{{}}
	for _, dimension := range dimensions {
		next := make([]map[string]any, 0, len(vectors)*len(dimension.values))
		for _, current := range vectors {
			for _, value := range dimension.values {
				vector := make(map[string]any, len(current)+1)
				for key, item := range current {
					vector[key] = item
				}
				vector[dimension.name] = value
				next = append(next, vector)
			}
		}
		vectors = next
	}
	return vectors, nil
}
