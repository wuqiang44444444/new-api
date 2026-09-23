package gemini

import (
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

var geminiOpenAPISchemaAllowedFields = map[string]struct{}{
	"anyOf":            {},
	"default":          {},
	"description":      {},
	"enum":             {},
	"example":          {},
	"format":           {},
	"items":            {},
	"maxItems":         {},
	"maxLength":        {},
	"maxProperties":    {},
	"maximum":          {},
	"minItems":         {},
	"minLength":        {},
	"minProperties":    {},
	"minimum":          {},
	"nullable":         {},
	"pattern":          {},
	"properties":       {},
	"propertyOrdering": {},
	"required":         {},
	"title":            {},
	"type":             {},
}

const geminiFunctionSchemaMaxDepth = 64

// maxGeminiSchemaDiagnostics bounds diagnostics per cleaning pass; a hostile
// schema must not grow request state without bound.
const maxGeminiSchemaDiagnostics = 16

// CleanFunctionParametersWithDiagnostics cleans with the exact semantics of the
// former CleanFunctionParameters and additionally returns bounded structural
// diagnostics for nodes that lose fields to the whitelist or keep no type.
// It never changes conversion semantics: no STRING default, no rejection.
func CleanFunctionParametersWithDiagnostics(params interface{}) (interface{}, []types.ConversionDiagnostic) {
	collector := &geminiSchemaDiagnosticCollector{}
	cleaned := cleanGeminiFunctionParametersWithDepth(params, 0, "", collector)
	return cleaned, collector.diagnostics
}

// geminiSchemaDiagnosticCollector bounds the diagnostics emitted by one
// cleaning pass.
type geminiSchemaDiagnosticCollector struct {
	diagnostics []types.ConversionDiagnostic
	truncated   bool
}

func (c *geminiSchemaDiagnosticCollector) add(path string, code, message string) {
	if len(c.diagnostics) >= maxGeminiSchemaDiagnostics {
		if !c.truncated {
			c.truncated = true
			c.diagnostics = append(c.diagnostics, types.ConversionDiagnostic{
				Code:     "gemini_schema_diagnostics_truncated",
				Path:     "",
				Message:  "further schema diagnostics truncated",
				Severity: types.ConversionDiagnosticWarning,
			})
		}
		return
	}
	c.diagnostics = append(c.diagnostics, types.ConversionDiagnostic{
		Code:     code,
		Path:     path,
		Message:  message,
		Severity: types.ConversionDiagnosticWarning,
	})
}

func cleanGeminiFunctionParametersWithDepth(params interface{}, depth int, path string, collector *geminiSchemaDiagnosticCollector) interface{} {
	if params == nil {
		return nil
	}

	if depth >= geminiFunctionSchemaMaxDepth {
		return cleanGeminiFunctionParametersShallow(params)
	}

	switch v := params.(type) {
	case map[string]interface{}:
		cleanedMap := make(map[string]interface{}, len(v))
		dropped := make([]string, 0, 4)
		for key, val := range v {
			if _, ok := geminiOpenAPISchemaAllowedFields[key]; ok {
				cleanedMap[key] = val
			} else {
				dropped = append(dropped, key)
			}
		}

		normalizeGeminiSchemaTypeAndNullable(cleanedMap)

		if len(dropped) > 0 {
			collector.add(path, "gemini_schema_fields_dropped", droppedFieldsMessage(dropped))
		}
		// anyOf-only nodes are valid unions without an explicit type; flagging
		// them would bury the real missing-type findings in false positives.
		if _, hasType := cleanedMap["type"]; !hasType {
			if _, hasUnion := cleanedMap["anyOf"]; !hasUnion {
				collector.add(path, "gemini_schema_type_missing", "schema node keeps no type after cleaning")
			}
		}

		if props, ok := cleanedMap["properties"].(map[string]interface{}); ok && props != nil {
			cleanedProps := make(map[string]interface{})
			for propName, propValue := range props {
				cleanedProps[propName] = cleanGeminiFunctionParametersWithDepth(propValue, depth+1, childPath(path, ".properties."+propName), collector)
			}
			cleanedMap["properties"] = cleanedProps
		}

		if items, ok := cleanedMap["items"].(map[string]interface{}); ok && items != nil {
			cleanedMap["items"] = cleanGeminiFunctionParametersWithDepth(items, depth+1, childPath(path, ".items"), collector)
		}
		if itemsArray, ok := cleanedMap["items"].([]interface{}); ok && len(itemsArray) > 0 {
			cleanedMap["items"] = cleanGeminiFunctionParametersWithDepth(itemsArray[0], depth+1, childPath(path, ".items[0]"), collector)
		}

		if nested, ok := cleanedMap["anyOf"].([]interface{}); ok && nested != nil {
			cleanedNested := make([]interface{}, len(nested))
			for i, item := range nested {
				cleanedNested[i] = cleanGeminiFunctionParametersWithDepth(item, depth+1, childPath(path, ".anyOf["+strconv.Itoa(i)+"]"), collector)
			}
			cleanedMap["anyOf"] = cleanedNested
		}

		return cleanedMap
	case []interface{}:
		cleanedArray := make([]interface{}, len(v))
		for i, item := range v {
			cleanedArray[i] = cleanGeminiFunctionParametersWithDepth(item, depth+1, childPath(path, "["+strconv.Itoa(i)+"]"), collector)
		}
		return cleanedArray
	default:
		return params
	}
}

// childPath appends one segment with a bounded total length so hostile schemas
// cannot grow diagnostic paths without bound.
func childPath(parent, segment string) string {
	path := parent + segment
	if len(path) > 128 {
		return path[:125] + "..."
	}
	return path
}

// droppedFieldsMessage lists only field NAMES of whitelist-dropped keys, never
// their values; bounded count and length.
func droppedFieldsMessage(dropped []string) string {
	names := make([]string, 0, len(dropped))
	for _, name := range dropped {
		trimmed := name
		if len(trimmed) > 32 {
			trimmed = trimmed[:32]
		}
		names = append(names, trimmed)
		if len(names) == 4 {
			break
		}
	}
	sort.Strings(names)
	message := "dropped: " + strings.Join(names, ",")
	if len(dropped) > 4 {
		message += ",..."
	}
	return message
}

func cleanGeminiFunctionParametersShallow(params interface{}) interface{} {
	switch v := params.(type) {
	case map[string]interface{}:
		cleanedMap := make(map[string]interface{}, len(v))
		for key, val := range v {
			if _, ok := geminiOpenAPISchemaAllowedFields[key]; ok {
				cleanedMap[key] = val
			}
		}
		normalizeGeminiSchemaTypeAndNullable(cleanedMap)
		delete(cleanedMap, "properties")
		delete(cleanedMap, "items")
		delete(cleanedMap, "anyOf")
		return cleanedMap
	case []interface{}:
		return []interface{}{}
	default:
		return params
	}
}

func normalizeGeminiSchemaTypeAndNullable(schema map[string]interface{}) {
	rawType, ok := schema["type"]
	if !ok || rawType == nil {
		return
	}

	normalize := func(t string) (string, bool) {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "object":
			return "OBJECT", false
		case "array":
			return "ARRAY", false
		case "string":
			return "STRING", false
		case "integer":
			return "INTEGER", false
		case "number":
			return "NUMBER", false
		case "boolean":
			return "BOOLEAN", false
		case "null":
			return "", true
		default:
			return t, false
		}
	}

	switch typed := rawType.(type) {
	case string:
		normalized, isNull := normalize(typed)
		if isNull {
			schema["nullable"] = true
			delete(schema, "type")
			return
		}
		schema["type"] = normalized
	case []interface{}:
		nullable := false
		var chosen string
		for _, item := range typed {
			if value, ok := item.(string); ok {
				normalized, isNull := normalize(value)
				if isNull {
					nullable = true
					continue
				}
				if chosen == "" {
					chosen = normalized
				}
			}
		}
		if nullable {
			schema["nullable"] = true
		}
		if chosen != "" {
			schema["type"] = chosen
		} else {
			delete(schema, "type")
		}
	}
}

func RemoveAdditionalProperties(schema interface{}, depth int) interface{} {
	if depth >= 5 {
		return schema
	}

	value, ok := schema.(map[string]interface{})
	if !ok || len(value) == 0 {
		return schema
	}
	delete(value, "title")
	delete(value, "$schema")
	if typeVal, exists := value["type"]; !exists || (typeVal != "object" && typeVal != "array") {
		return schema
	}
	switch value["type"] {
	case "object":
		delete(value, "additionalProperties")
		if properties, ok := value["properties"].(map[string]interface{}); ok {
			for key, nested := range properties {
				properties[key] = RemoveAdditionalProperties(nested, depth+1)
			}
		}
		for _, field := range []string{"allOf", "anyOf", "oneOf"} {
			if nested, ok := value[field].([]interface{}); ok {
				for i, item := range nested {
					nested[i] = RemoveAdditionalProperties(item, depth+1)
				}
			}
		}
	case "array":
		if items, ok := value["items"].(map[string]interface{}); ok {
			value["items"] = RemoveAdditionalProperties(items, depth+1)
		}
	}

	return value
}

func OpenAIToolChoiceToConfig(toolChoice any) *dto.ToolConfig {
	if toolChoice == nil {
		return nil
	}

	if toolChoiceStr, ok := toolChoice.(string); ok {
		config := &dto.ToolConfig{
			FunctionCallingConfig: &dto.FunctionCallingConfig{},
		}
		switch toolChoiceStr {
		case "auto":
			config.FunctionCallingConfig.Mode = "AUTO"
		case "none":
			config.FunctionCallingConfig.Mode = "NONE"
		case "required":
			config.FunctionCallingConfig.Mode = "ANY"
		default:
			config.FunctionCallingConfig.Mode = "AUTO"
		}
		return config
	}

	if toolChoiceMap, ok := toolChoice.(map[string]interface{}); ok {
		if toolChoiceMap["type"] == "function" {
			config := &dto.ToolConfig{
				FunctionCallingConfig: &dto.FunctionCallingConfig{
					Mode: "ANY",
				},
			}
			if function, ok := toolChoiceMap["function"].(map[string]interface{}); ok {
				if name, ok := function["name"].(string); ok && name != "" {
					config.FunctionCallingConfig.AllowedFunctionNames = []string{name}
				}
			}
			return config
		}
		return nil
	}

	return nil
}
