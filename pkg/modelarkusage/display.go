package modelarkusage

import (
	"encoding/json"
	"math"

	"github.com/QuantumNous/new-api/common"
)

// ObservationKey is removed from normalized poll responses before saving Task.Data.
const ObservationKey = "_modelark_display_usage"

// Display holds additional display measurements. A nil field is unobserved.
// The billing facts retain their own sources and validation.
type Display struct {
	TotalSeconds      *float64 `json:"total_seconds,omitempty"`
	InputAudioSeconds *float64 `json:"input_audio_seconds,omitempty"`
	PromptTokens      *int     `json:"prompt_tokens,omitempty"`
	WebSearch         *int     `json:"web_search,omitempty"`
}

func Merge(accepted, observed *Display) *Display {
	if observed == nil {
		return accepted
	}
	var next Display
	if accepted != nil {
		next = *accepted
	}
	if next.TotalSeconds == nil {
		next.TotalSeconds = observed.TotalSeconds
	}
	if next.InputAudioSeconds == nil {
		next.InputAudioSeconds = observed.InputAudioSeconds
	}
	if next.PromptTokens == nil {
		next.PromptTokens = observed.PromptTokens
	}
	if next.WebSearch == nil {
		next.WebSearch = observed.WebSearch
	}
	if next == (Display{}) {
		return nil
	}
	return &next
}

func Seconds(value any, max float64) *float64 {
	if seconds, ok := value.(float64); ok && seconds >= 0 && seconds <= max {
		return &seconds
	}
	return nil
}

func Counter(value any) *int {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || number < 0 || number > math.MaxInt32 || number != math.Trunc(number) {
		return nil
	}
	count := int(number)
	return &count
}

// RawWebSearch reads the standard ModelArk tool_usage object.
func RawWebSearch(raw json.RawMessage) *int {
	var tools struct {
		WebSearch json.RawMessage `json:"web_search"`
	}
	if common.Unmarshal(raw, &tools) != nil {
		return nil
	}
	return RawCounter(tools.WebSearch)
}

func RawCounter(raw json.RawMessage) *int {
	var count *int
	if common.Unmarshal(raw, &count) == nil && count != nil && *count >= 0 && int64(*count) <= math.MaxInt32 {
		return count
	}
	return nil
}
