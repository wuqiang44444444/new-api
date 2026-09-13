package main

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationPreservesWholeAmountAndNumericConditions(t *testing.T) {
	schema := seedancebilling.UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3CMCC)
	for _, legacy := range []string{
		`param("_task.resolution") == "1080p" ? tier("fixed", 1000) : tier("base", c * 5)`,
		`tier("seconds", param("_task.duration_seconds") * 400000)`,
		`c > 200000 ? tier("high", c * 5 + 100000) : tier("low", c * 4 + 100000)`,
		`v1:tier("中文", c * 2.346041055718 + 1000.0)`,
	} {
		t.Run(legacy, func(t *testing.T) {
			candidate, err := convertLegacyExpression(legacy, schema)
			require.NoError(t, err)
			require.NoError(t, seedancebilling.ValidateTaskExpression(candidate, schema))
			for _, resolution := range []string{"480p", "720p", "1080p", "4k"} {
				for _, tokens := range []int{0, 1, 199999, 200000, 200001, 300000} {
					body, err := common.Marshal(map[string]any{"_task": map[string]any{"resolution": resolution, "duration_seconds": 5}})
					require.NoError(t, err)
					old, oldTrace, err := billingexpr.RunExprWithRequest(legacy, billingexpr.TokenParams{C: float64(tokens)}, billingexpr.RequestInput{Body: body})
					require.NoError(t, err)
					facts, err := seedancebilling.ControlledFacts(body, tokens)
					require.NoError(t, err)
					got, trace, err := billingexpr.RunExprWithRequest(candidate, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
					require.NoError(t, err)
					assert.Equal(t, old/1000000, got, "resolution=%s tokens=%d candidate=%s", resolution, tokens, candidate)
					assert.Equal(t, oldTrace.MatchedTier, trace.MatchedTier)
				}
			}
		})
	}
}
func TestEquivalenceRejectsWrongMiddleEnumBranch(t *testing.T) {
	schema := seedancebilling.UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3CMCC)
	vectors, err := proofVectors(`u("resolution") == "1080p" ? tier("fixed", 1000) : tier("base", u("tokens") * 5 / 1000000)`, schema, 300000, true)
	require.NoError(t, err)
	legacy := `param("_task.resolution") == "1080p" ? tier("fixed", 1000) : tier("base", c * 5)`
	wrong := `u("resolution") == "1080p" ? tier("fixed", 1000) : tier("base", u("tokens") * 5 / 1000000)`
	require.NotEmpty(t, proveEquivalence(legacy, wrong, vectors, []float64{1}))
}

func TestMigrationProofDoesNotHideDifferencesBehindQuotaSaturation(t *testing.T) {
	vectors := []proofVector{{ProbeBody: []byte(`{"_task":{}}`), Tokens: 1}}
	require.NotEmpty(t, proveEquivalence(`tier("base", 1000000000000)`, `tier("base", 2000000)`, vectors, []float64{1}))
	require.NotEmpty(t, proveEquivalence(`tier("base", 1)`, `tier("base", 1)`, nil, []float64{1}))
}

func TestMigrationRejectsUnprovenInputs(t *testing.T) {
	schema := seedancebilling.UsageFieldsForProtocol(dto.VideoUpstreamProtocolModelArkV3CMCC)
	for _, expression := range []string{
		`tier("fixed", 500000)`,
		`tier("fixed", 0.5)`,
		`(tier("fixed", 500000)) / 1000000`,
		`let meter = param; tier("base", meter("_task.duration_seconds"))`,
		`tier("base", param("_task.size_multiplier") * c)`,
		`tier("base", u("tokens") * 5 / 1000000)`,
		`tier("base", hour() * c)`,
	} {
		_, err := convertLegacyExpression(expression, schema)
		require.Error(t, err, expression)
	}
}

func TestSimpleProtocolPricesDoNotEnumerateUnreferencedFields(t *testing.T) {
	for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolFunCloudModelArkV3, dto.VideoUpstreamProtocolFeicaiVideosV1} {
		for _, tc := range []struct {
			expression string
			count      int
		}{
			{`tier("tokens", c * 5)`, 5},
			{`tier("seconds", param("_task.duration_seconds") * 400000)`, 4},
		} {
			t.Run(string(protocol)+tc.expression, func(t *testing.T) {
				schema := seedancebilling.UsageFieldsForProtocol(protocol)
				candidate, err := convertLegacyExpression(tc.expression, schema)
				require.NoError(t, err)
				require.NoError(t, seedancebilling.ValidateTaskExpression(candidate, schema))
				vectors, err := proofVectors(candidate, schema, 300000, true)
				require.NoError(t, err)
				assert.Len(t, vectors, tc.count)
				assert.Empty(t, proveEquivalence(tc.expression, candidate, vectors, []float64{0, 1, 0.87}))
			})
		}
	}
}
