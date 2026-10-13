package gemini

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCleanFunctionParametersPreservesGeminiSchemaContract(t *testing.T) {
	input := map[string]interface{}{
		"type": "object", "additionalProperties": true,
		"properties": map[string]interface{}{
			"name":  map[string]interface{}{"type": []interface{}{"string", "null"}},
			"union": map[string]interface{}{"anyOf": []interface{}{map[string]interface{}{"type": "string"}, map[string]interface{}{"type": "integer"}}},
		},
	}
	cleaned, ok := CleanFunctionParameters(input).(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "OBJECT", cleaned["type"])
	assert.NotContains(t, cleaned, "additionalProperties")
	properties, ok := cleaned["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, map[string]interface{}{"type": "STRING", "nullable": true}, properties["name"])
	union, ok := properties["union"].(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, union, "type", "anyOf-only nodes must retain their union shape")
	assert.Equal(t, []interface{}{map[string]interface{}{"type": "STRING"}, map[string]interface{}{"type": "INTEGER"}}, union["anyOf"])
	assert.Contains(t, input, "additionalProperties", "cleaning must not mutate the caller's schema")
}
