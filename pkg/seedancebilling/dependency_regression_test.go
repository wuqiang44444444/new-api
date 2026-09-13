package seedancebilling

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTaskExpressionRejectsIndirectUsageReferences(t *testing.T) {
	schema := UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3CMCC)
	for _, expression := range []string{
		`let meter = u; tier("base", meter("tokens") + (u("resolution") == "4k" ? 1 : 0))`,
		`tier("base", u("duration_seconds") + u(param("meter")))`,
		`tier("base", u("tokens ") * 5 / 1000000)`,
	} {
		t.Run(expression, func(t *testing.T) {
			require.Error(t, ValidateTaskExpression(expression, schema))
			assert.True(t, RequiresMeasuredTokens(expression), "unproven references cannot authorize settlement")
		})
	}
}

func TestTaskExpressionValidatesMiddleConditionCombinations(t *testing.T) {
	schema := UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3CMCC)
	expression := `u("duration_seconds") == 1 && u("generate_audio") && u("resolution") == "1080p" ? tier("bad", -1) : tier("base", 0.5)`
	require.ErrorContains(t, ValidateTaskExpression(expression, schema), "non-negative")
}
