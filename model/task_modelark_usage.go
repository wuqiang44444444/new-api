package model

import (
	"math"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/minimaxlink"
	"github.com/QuantumNous/new-api/pkg/modelarkusage"
)

func (t *Task) mergeModelArkDisplayUsage(observed *modelarkusage.Display) {
	accepted := t.PrivateData.ModelArkUsage
	if t.IsMiniMaxV2Task() {
		accepted = validMiniMaxDisplayUsage(t.PrivateData.AsyncBilling, accepted)
		observed = validMiniMaxDisplayUsage(t.PrivateData.AsyncBilling, observed)
	}
	t.PrivateData.ModelArkUsage = modelarkusage.Merge(accepted, observed)
}

func validMiniMaxDisplayUsage(async *TaskAsyncBillingContext, display *modelarkusage.Display) *modelarkusage.Display {
	if display == nil || display.TotalSeconds == nil || validMiniMaxDisplayTotal(async, *display.TotalSeconds) {
		return display
	}
	next := *display
	next.TotalSeconds = nil
	return modelarkusage.Merge(nil, &next)
}

// The accepted media facts narrow the interval allowed by the frozen request.
// This check applies to both new observations and previously stored candidates.
func validMiniMaxDisplayTotal(async *TaskAsyncBillingContext, total float64) bool {
	if async == nil || async.TieredSnapshot == nil {
		return false
	}
	var lower, upper float64
	for _, field := range []string{minimaxlink.OfficialInputVideoSeconds, minimaxlink.OfficialOutputSeconds} {
		if value, known := async.OfficialUsage[field]; known {
			lower += value
			upper += value
			continue
		}
		budget, known := async.TieredSnapshot.UsageFacts[field].(float64)
		if !known || !minimaxlink.ValidOfficialMeasurement(field, budget) {
			return false
		}
		if field == minimaxlink.OfficialOutputSeconds {
			lower += minimaxlink.OfficialMinOutputSeconds
		}
		upper += budget
	}
	return total >= math.Nextafter(lower, math.Inf(-1)) && total <= math.Nextafter(upper, math.Inf(1))
}

func (t *Task) projectModelArkDisplayUsage(usage *dto.ModelArkVideoTaskUsage) {
	display := t.PrivateData.ModelArkUsage
	if display != nil {
		if display.InputAudioSeconds != nil {
			usage.InputAudioSeconds = *display.InputAudioSeconds
		}
		if display.PromptTokens != nil {
			usage.PromptTokens = *display.PromptTokens
		}
		if display.WebSearch != nil {
			usage.ToolUsage.WebSearch = *display.WebSearch
		}
	}

	async := t.PrivateData.AsyncBilling
	if !t.IsMiniMaxV2Task() || async == nil {
		return
	}
	input, hasInput := async.OfficialUsage[minimaxlink.OfficialInputVideoSeconds]
	output, hasOutput := async.OfficialUsage[minimaxlink.OfficialOutputSeconds]
	usage.InputSeconds, usage.OutputSeconds = input, output
	usage.InputImageCount = int(async.OfficialUsage[minimaxlink.OfficialInputImageCount])
	if !hasInput && async.TieredSnapshot != nil {
		budget, known := async.TieredSnapshot.UsageFacts[minimaxlink.OfficialInputVideoSeconds].(float64)
		hasInput = known && budget == 0
	}
	if hasInput && hasOutput {
		usage.TotalSeconds = input + output
	} else if display != nil && display.TotalSeconds != nil && validMiniMaxDisplayTotal(async, *display.TotalSeconds) {
		usage.TotalSeconds = *display.TotalSeconds
	}
}
