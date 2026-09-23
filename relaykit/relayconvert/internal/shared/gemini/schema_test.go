package gemini

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cleaning keeps whitelist fields and normalizes types exactly as before the
// diagnostics pass; the output must not change with the collector attached.
func TestCleanFunctionParametersWithDiagnosticsPreservesCleaning(t *testing.T) {
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": []interface{}{"string", "null"}},
			"age":  map[string]interface{}{"type": "integer"},
		},
		"additionalProperties": true,
	}
	cleaned, diagnostics := CleanFunctionParametersWithDiagnostics(input)
	cleanedMap, ok := cleaned.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "OBJECT", cleanedMap["type"])
	props := cleanedMap["properties"].(map[string]interface{})
	name := props["name"].(map[string]interface{})
	assert.Equal(t, "STRING", name["type"])
	assert.Equal(t, true, name["nullable"])
	// Dropped field is reported, but only by name and never by value.
	assert.Equal(t, "dropped: additionalProperties", diagnostics[0].Message)
	assert.Empty(t, diagnostics[0].Path)
}

// A node that keeps no type after cleaning is reported with its structural
// path; no default type is injected and the schema passes through unchanged.
func TestCleanFunctionParametersWithDiagnosticsReportsMissingType(t *testing.T) {
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"video": map[string]interface{}{
				"description": "the video",
				"enum":        []interface{}{"a", "b"},
			},
		},
	}
	_, diagnostics := CleanFunctionParametersWithDiagnostics(input)
	var missing []string
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "gemini_schema_type_missing" {
			missing = append(missing, diagnostic.Path)
		}
	}
	assert.Contains(t, missing, ".properties.video")
	assert.NotContains(t, missing, "")
}

// anyOf-only union nodes are valid without an explicit type and must not be
// flagged as missing-type; the diagnostic stays reserved for real findings.
func TestCleanFunctionParametersWithDiagnosticsAllowsAnyOfOnlyNodes(t *testing.T) {
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"union": map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}
	_, diagnostics := CleanFunctionParametersWithDiagnostics(input)
	for _, diagnostic := range diagnostics {
		assert.NotEqual(t, "gemini_schema_type_missing", diagnostic.Code)
	}
}

// Reference structures dropped by the whitelist are reported with field names
// only; the diagnostic message must never carry schema values.
func TestCleanFunctionParametersWithDiagnosticsReportsDroppedRefFields(t *testing.T) {
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ref": map[string]interface{}{"$ref": "#/components/schemas/Secret"},
			"union": map[string]interface{}{
				"allOf": []interface{}{map[string]interface{}{"type": "string"}},
			},
		},
	}
	_, diagnostics := CleanFunctionParametersWithDiagnostics(input)
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
		assert.NotContains(t, diagnostic.Message, "Secret", "field values must never enter diagnostics")
	}
	assert.Contains(t, messages, "dropped: $ref")
	assert.Contains(t, messages, "dropped: allOf")
	paths := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		paths = append(paths, diagnostic.Path)
	}
	assert.Contains(t, paths, ".properties.ref")
	assert.Contains(t, paths, ".properties.union")
}

// Diagnostics stay bounded: many lossy nodes produce the truncation marker once.
func TestCleanFunctionParametersWithDiagnosticsBoundedAndTruncated(t *testing.T) {
	properties := map[string]interface{}{}
	for i := 0; i < 40; i++ {
		properties[strings.Repeat("p", i+1)] = map[string]interface{}{"description": "lossy"}
	}
	input := map[string]interface{}{"type": "object", "properties": properties}
	_, diagnostics := CleanFunctionParametersWithDiagnostics(input)
	require.Less(t, len(diagnostics), 20)
	truncated := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "gemini_schema_diagnostics_truncated" {
			truncated++
		}
	}
	assert.Equal(t, 1, truncated)
}
