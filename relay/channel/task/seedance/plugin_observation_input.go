package seedance

import (
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"strings"
)

// URL syntax is a host transport constraint, while candidate selection remains
// in the versioned adapter. Enumerate valid strings without interpreting any
// Provider field, status or model. Embedded JSON strings are bounded as well.
func seedanceObservationInput(body []byte, expectedID string) map[string]any {
	input := map[string]any{"taskId": expectedID, "body": string(body)}
	var root any
	valid := make(map[string]bool)
	if common.Unmarshal(body, &root) == nil {
		collectValidVideoResultURLs(root, 0, valid)
	}
	input["validResultURLs"] = valid
	return input
}
func collectValidVideoResultURLs(value any, depth int, valid map[string]bool) {
	if depth > 32 {
		return
	}
	switch v := value.(type) {
	case string:
		if url, err := relaycommon.ValidateHTTPSVideoResultURL(v); err == nil {
			valid[url] = true
			return
		}
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var nested any
			if common.Unmarshal([]byte(trimmed), &nested) == nil {
				collectValidVideoResultURLs(nested, depth+1, valid)
			}
		}
	case map[string]any:
		for _, item := range v {
			collectValidVideoResultURLs(item, depth+1, valid)
		}
	case []any:
		for _, item := range v {
			collectValidVideoResultURLs(item, depth+1, valid)
		}
	}
}
