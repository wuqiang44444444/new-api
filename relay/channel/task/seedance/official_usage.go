package seedance

import (
	"encoding/json"
	"math"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Official ModelArk bills completion_tokens. A total or prompt count alone
// cannot substitute for that field. Invalid/missing usage leaves a valid video
// result successful so the durable usage worker can collect later evidence.
func normalizeOfficialTaskUsage(body []byte, expectedID string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk task response"}
	}
	var id, status string
	if common.Unmarshal(payload["id"], &id) != nil || id != expectedID || id == "" {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "ModelArk task id mismatch"}
	}
	if err := common.Unmarshal(payload["status"], &status); err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk task status"}
	}
	if status != "succeeded" {
		return body, nil
	}
	var raw map[string]json.RawMessage
	_ = common.Unmarshal(payload["usage"], &raw)
	usage := map[string]int{}
	evidence := map[string]int{}
	for _, key := range []string{"completion_tokens", "total_tokens", "prompt_tokens"} {
		value, exists := raw[key]
		if !exists || string(value) == "null" {
			continue
		}
		var amount int64
		if common.Unmarshal(value, &amount) != nil || amount < 0 || amount > math.MaxInt32 {
			continue
		}
		evidence["usage."+key] = int(amount)
		usage[key] = int(amount)
	}
	delete(payload, "usage")
	delete(payload, "usage_source")
	delete(payload, "usage_evidence")
	if completion, ok := usage["completion_tokens"]; ok {
		if _, ok := usage["total_tokens"]; !ok {
			usage["total_tokens"] = completion
		}
		payload["usage"], _ = common.Marshal(usage)
		payload["usage_source"], _ = common.Marshal("usage.completion_tokens")
	}
	if len(evidence) > 0 {
		payload["usage_evidence"], _ = common.Marshal(evidence)
	}
	return common.Marshal(payload)
}
