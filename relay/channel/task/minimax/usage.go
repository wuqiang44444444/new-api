package minimax

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Registered usage-subtree locators. The plugin locates the provider usage
// field; the host reads the value from the raw observation and validates it.
var creditUsageScanRoots = map[string]func(map[string]json.RawMessage) (json.RawMessage, bool){
	"usage.video_output":         resolveTopLevelCreditField,
	"content.usage.video_output": resolveNestedCreditField,
}

// creditUsageField is the evidence key under which the located value is
// stored. The value keeps its official JD Cloud meaning: provider credits,
// never seconds and never tokens.
const creditUsageField = "video_output"

// CreditUsageSource labels the evidence unit and source field.
const CreditUsageSource = "credit:" + creditUsageField

// MaxCreditValue saturates absurd upstream numbers. It is an evidence bound,
// not a billing multiplier.
const MaxCreditValue = 1000000000

// NormalizeCreditUsage reads the plugin-located usage subtree from the raw
// observation bytes and forms provider usage evidence. The credit value is
// validated in the host (integer, non-negative, bounded); an absent or
// malformed field forms no evidence and never fails the observation. The
// result is never written into token fields: token normalization keeps its
// own entry point.
func NormalizeCreditUsage(rawBody []byte, scanRoot string) (evidence map[string]int, source string) {
	locator, registered := creditUsageScanRoots[strings.TrimSpace(scanRoot)]
	if !registered {
		return nil, ""
	}
	var root map[string]json.RawMessage
	if err := common.Unmarshal(rawBody, &root); err != nil {
		return nil, ""
	}
	raw, ok := locator(root)
	if !ok {
		return nil, ""
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return nil, ""
	}
	if value < 0 || value > MaxCreditValue {
		return nil, ""
	}
	return map[string]int{creditUsageField: int(value)}, CreditUsageSource
}

func resolveTopLevelCreditField(root map[string]json.RawMessage) (json.RawMessage, bool) {
	return resolveCreditFieldIn(root)
}

func resolveNestedCreditField(root map[string]json.RawMessage) (json.RawMessage, bool) {
	var content []map[string]json.RawMessage
	if err := common.Unmarshal(root["content"], &content); err != nil || len(content) != 1 {
		return nil, false
	}
	return resolveCreditFieldIn(content[0])
}

func resolveCreditFieldIn(object map[string]json.RawMessage) (json.RawMessage, bool) {
	var usage struct {
		VideoOutput json.RawMessage `json:"video_output"`
	}
	if err := common.Unmarshal(object["usage"], &usage); err != nil {
		return nil, false
	}
	if len(usage.VideoOutput) == 0 {
		return nil, false
	}
	return usage.VideoOutput, true
}
