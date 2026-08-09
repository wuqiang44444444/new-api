package model

import "errors"

var ErrModelMappingCycle = errors.New("model mapping contains a cycle")

// ResolveModelMapping follows NEWAPI's ordinary model_mapping chain without
// normalizing model-name literals. mappingApplied reports whether the chain
// consumed at least one non-empty mapping entry, including a self-mapping.
func ResolveModelMapping(modelName string, mapping map[string]string) (string, bool, error) {
	current := modelName
	visited := map[string]struct{}{current: {}}
	mappingApplied := false
	for {
		next, exists := mapping[current]
		if !exists || next == "" {
			return current, mappingApplied, nil
		}
		mappingApplied = true
		if next == current {
			return current, mappingApplied, nil
		}
		if _, exists := visited[next]; exists {
			return "", false, ErrModelMappingCycle
		}
		visited[next] = struct{}{}
		current = next
	}
}
