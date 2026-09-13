package seedancebilling

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageFieldsForProtocolDeclaresCommonAndProtocolFields(t *testing.T) {
	common := UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3Volcengine)
	for _, name := range []string{"tokens", "resolution", "has_video_input", "duration_seconds", "generate_audio", "input_mode", "control_mode"} {
		assert.Contains(t, common, name)
	}
	assert.NotContains(t, common, "ratio")
	assert.NotContains(t, common, "billing_mode")
	assert.NotContains(t, common, "size_multiplier")

	feicaiSchema := UsageFieldsForProtocol(dto.VideoUpstreamProtocolFeicaiVideosV1)
	assert.Contains(t, feicaiSchema, "ratio")
	assert.Contains(t, feicaiSchema, "billing_mode")
	assert.Equal(t, []string{"per-second"}, feicaiSchema["billing_mode"].Enum)
	assert.NotContains(t, feicaiSchema, "size_multiplier")

	funcloud := UsageFieldsForProtocol(dto.VideoUpstreamProtocolFunCloudModelArkV3)
	assert.Equal(t, []string{"per-second", "per-token"}, funcloud["billing_mode"].Enum)
	assert.NotContains(t, funcloud, "ratio")

	assert.Equal(t, "token", common["tokens"].Unit)
	assert.Equal(t, "second", common["duration_seconds"].Unit)
	assert.Equal(t, "boolean", common["has_video_input"].Type)
}

func TestCMCCPricingValidatesOnlySupportedResolutions(t *testing.T) {
	expression := `tier("official", u("tokens") * (u("has_video_input") ? {"480p": 56, "720p": 56, "1080p": 62}[u("resolution")] : {"480p": 92, "720p": 92, "1080p": 102}[u("resolution")]) / 7000000)`
	require.NoError(t, ValidateTaskExpression(expression, UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3CMCC)))
	// Other protocols still need a price for their advertised 4k requests.
	require.Error(t, ValidateTaskExpression(expression, UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3Volcengine)))
}

func TestTokenBudgetRequirementPreservesProtocolBillingContract(t *testing.T) {
	for _, tc := range []struct {
		name       string
		protocol   dto.VideoUpstreamProtocol
		expression string
		required   bool
	}{
		{"token protocol fixed expression", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", 0.5)`, true},
		{"synlink fixed expression", dto.VideoUpstreamProtocolSynlinkVideoV1, `tier("base", 0.5)`, false},
		{"funcloud frozen duration", dto.VideoUpstreamProtocolFunCloudModelArkV3, `tier("base", u("duration_seconds") * 0.1)`, false},
		{"synlink measured tokens", dto.VideoUpstreamProtocolSynlinkVideoV1, `tier("base", u("tokens") / 1000000)`, true},
		{"funcloud measured tokens", dto.VideoUpstreamProtocolFunCloudModelArkV3, `tier("base", u("tokens") / 1000000)`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.required, RequiresTokenBudget(tc.protocol, tc.expression))
		})
	}
}

func TestIntersectUsageFieldsDropsProtocolOnlyFields(t *testing.T) {
	merged := IntersectUsageFields(
		UsageFieldsForProtocol(dto.VideoUpstreamProtocolFeicaiVideosV1),
		UsageFieldsForProtocol(dto.VideoUpstreamProtocolFunCloudModelArkV3),
	)
	assert.Contains(t, merged, "tokens")
	assert.Contains(t, merged, "billing_mode")
	assert.Equal(t, []string{"per-second"}, merged["billing_mode"].Enum)
	assert.NotContains(t, merged, "ratio")
}

func TestUsageUnitsForSchemaClassifiesStatementUnits(t *testing.T) {
	units := UsageUnitsForSchema(UsageFieldsForProtocol(dto.VideoUpstreamProtocolFunCloudModelArkV3))
	assert.Equal(t, "token", units["tokens"])
	assert.Equal(t, "enum", units["resolution"])
	assert.Equal(t, "boolean", units["has_video_input"])
	assert.Equal(t, "second", units["duration_seconds"])
	assert.Equal(t, "enum", units["billing_mode"])
}

func TestRequiresMeasuredTokensDistinguishesMeterFromFrozenConditions(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{"token meter", `tier("base", u("tokens") * 5 / 1000000)`, true},
		{"token with conditions", `u("resolution") == "4k" ? tier("4k", u("tokens") * 8 / 1000000) : tier("base", u("tokens") * 5 / 1000000)`, true},
		{"frozen duration only", `tier("base", u("duration_seconds") * 0.4)`, false},
		{"frozen resolution only", `u("resolution") == "4k" ? tier("4k", 0.7) : tier("base", 0.4)`, false},
		{"mixed conditions without meter", `u("has_video_input") ? tier("video", u("duration_seconds") * 0.5) : tier("image", 0.2)`, false},
		{"legacy token expression", `tier("base", c * 5)`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, RequiresMeasuredTokens(test.expression))
		})
	}
}

func TestStaticUsageKeysResolvesFoldedConstants(t *testing.T) {
	// expr-lang constant-folds "resolu" + "tion" into a literal, so the static
	// analysis still proves the dependency instead of failing closed.
	assert.False(t, RequiresMeasuredTokens(`tier("base", u("resolu" + "tion") * 5 / 1000000)`), "the folded key resolves to a frozen condition, not the meter")
	assert.True(t, RequiresMeasuredTokens(`tier("base", u("toke" + "ns") * 5 / 1000000)`), "the folded key resolves to the measured meter")
}

func TestRequiresMeasuredTaskUsageDispatchesOnFrozenUnitContract(t *testing.T) {
	usageSnapshot := &billingexpr.BillingSnapshot{TaskUsageBilling: true, ExprString: `tier("base", u("resolution") == "4k" ? 0.7 : 0.4)`}
	assert.False(t, RequiresMeasuredTaskUsage(usageSnapshot), "frozen-condition u() expression must not wait for measured tokens")

	usageSnapshotWithTokens := &billingexpr.BillingSnapshot{TaskUsageBilling: true, ExprString: `tier("base", u("tokens") * 5 / 1000000)`}
	assert.True(t, RequiresMeasuredTaskUsage(usageSnapshotWithTokens))

	legacySnapshot := &billingexpr.BillingSnapshot{ExprString: `tier("base", c * 5)`}
	assert.True(t, RequiresMeasuredTaskUsage(legacySnapshot), "legacy token expressions keep the generic usage dependency")

	legacyFrozenSnapshot := &billingexpr.BillingSnapshot{ExprString: `tier("base", 0.5)`}
	assert.False(t, RequiresMeasuredTaskUsage(legacyFrozenSnapshot))
}

func TestControlledFactsProjectsProbeAndMeter(t *testing.T) {
	probeBody := []byte(`{"_task":{"resolution":"1080p","has_video_input":true,"duration_seconds":5,"generate_audio":false,"input_mode":"text","control_mode":"none","size_multiplier":1}}`)
	facts, err := ControlledFacts(probeBody, 325000)
	require.NoError(t, err)
	assert.Equal(t, "1080p", facts["resolution"])
	assert.Equal(t, true, facts["has_video_input"])
	assert.Equal(t, float64(5), facts["duration_seconds"])
	assert.Equal(t, float64(325000), facts["tokens"])

	empty, err := ControlledFacts(nil, 0)
	require.NoError(t, err)
	assert.Equal(t, float64(0), empty["tokens"])

	_, err = ControlledFacts([]byte(`{invalid`), 0)
	require.ErrorContains(t, err, "frozen task billing probe")
}

func TestValidateTaskExpressionAcceptsContractExpressions(t *testing.T) {
	schema := UsageFieldsForProtocol(dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.NoError(t, ValidateTaskExpression(`tier("base", u("tokens") * 5 / 1000000)`, schema))
	require.NoError(t, ValidateTaskExpression(`u("resolution") == "4k" ? tier("4k", u("tokens") * 8 / 1000000) : tier("base", u("tokens") * 5 / 1000000)`, schema))
	require.NoError(t, ValidateTaskExpression(`tier("base", u("duration_seconds") * 0.4)`, schema))
	require.NoError(t, ValidateTaskExpression(`tier("1080p_ratio", u("duration_seconds") * 0.4) * (u("ratio") == "21:9" ? 2 : 1)`, schema), "request-rule fields declared by the protocol stay valid")
}

func TestValidateTaskExpressionRejectsContractViolations(t *testing.T) {
	schema := UsageFieldsForProtocol(dto.VideoUpstreamProtocolFeicaiVideosV1)
	tests := []struct {
		name       string
		expression string
		message    string
	}{
		{"undeclared key", `tier("base", u("size_multiplier") * 5 / 1000000)`, "not declared"},
		{"legacy token var", `tier("base", u("tokens") * 5 / 1000000 + c * 1)`, "token expression contract"},
		{"frozen task param", `tier("base", param("_task.duration_seconds") * 0.4)`, "declared u() fields"},
		{"header function", `tier("base", u("tokens") * 5 / 1000000) * (has(header("x-tier"), "fast") ? 2 : 1)`, "non-deterministic"},
		{"time function", `tier("base", u("tokens") * 5 / 1000000) * (hour("UTC") >= 0 ? 1 : 1)`, "non-deterministic"},
		{"missing tier wrap", `u("tokens") * 5 / 1000000`, "tier(name, value)"},
		{"negative result", `tier("base", 0 - u("duration_seconds"))`, "non-negative"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTaskExpression(test.expression, schema)
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestValidateTaskExpressionAllowsEmptyVectorFreeContracts(t *testing.T) {
	schema := UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3Volcengine)
	assert.NoError(t, ValidateTaskExpression(`tier("base", 0.5)`, schema))
}

func TestControlledFactsTokenBudgetNeverExceedsConfiguredBound(t *testing.T) {
	// The budget is administrator-configured and already bounded by
	// MaxTaskPreConsumeTokens; this pins the projection contract that the
	// meter lands under the frozen key verbatim.
	const bound = math.MaxInt32 / 2
	facts, err := ControlledFacts([]byte(`{"_task":{}}`), bound)
	require.NoError(t, err)
	assert.Equal(t, float64(bound), facts["tokens"])
}
