package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

func rejectUnknownVideoFields(body map[string]any, fields ...string) error {
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	for field := range body {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unsupported parameter %q", field)
		}
	}
	return nil
}

func rejectUnknownNestedVideoFields(value any, path string, fields ...string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", path)
	}
	if err := rejectUnknownVideoFields(object, fields...); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func rejectUnknownNestedVideoArrayFields(value any, path string, fields ...string) error {
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array", path)
	}
	for index, item := range items {
		if err := rejectUnknownNestedVideoFields(item, fmt.Sprintf("%s[%d]", path, index), fields...); err != nil {
			return err
		}
	}
	return nil
}

func decodeTypedVideoRequest(body map[string]any, target any) error {
	encoded, err := common.Marshal(body)
	if err != nil {
		return err
	}
	return common.Unmarshal(encoded, target)
}

func videoStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
