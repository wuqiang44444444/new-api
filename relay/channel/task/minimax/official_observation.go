package minimax

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/minimaxlink"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// decodeOfficialTaskObservation reads measured amounts from the original V2
// response. The artifact identifies the task and locates the result, but its
// output is never used as a pricing multiplier.
func decodeOfficialTaskObservation(task *model.Task, result any, rawBody []byte, expectedTaskID string) (*normalizedObservation, []byte, error) {
	observation, normalized, err := decodeTaskObservation(result, rawBody, expectedTaskID)
	if err != nil {
		return observation, normalized, err
	}
	var raw struct {
		Task map[string]any `json:"task"`
	}
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid MiniMax V2 task body"}
	}
	status, _ := raw.Task["status"].(string)
	if task.PrivateData.VideoUpstreamProtocol == constant.VideoUpstreamProtocolNexhinaHailuoV2 && status == "processing" {
		status = observationStatusRunning
	}
	if raw.Task["id"] != expectedTaskID || status != observation.Status {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid MiniMax V2 task identity or status"}
	}
	var stored map[string]any
	if err := common.Unmarshal(normalized, &stored); err != nil {
		return nil, nil, fmt.Errorf("invalid normalized MiniMax official observation")
	}
	delete(stored, "id")
	if observation.Status != observationStatusSucceeded {
		normalized, err = common.Marshal(stored)
		return observation, normalized, err
	}
	content, _ := raw.Task["content"].(map[string]any)
	if content["url"] != observation.VideoURL {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid MiniMax official video result source"}
	}
	reportedResolution, _ := raw.Task["resolution"].(string)
	if resolution := strings.ToLower(reportedResolution); resolution == "768p" || resolution == "2k" {
		stored["resolution"] = resolution
	}
	if ratio, _ := raw.Task["ratio"].(string); ratio == "16:9" || ratio == "21:9" || ratio == "4:3" || ratio == "1:1" || ratio == "3:4" || ratio == "9:16" {
		stored["ratio"] = ratio
	}
	if duration, ok := raw.Task["duration"].(float64); ok &&
		minimaxlink.ValidOfficialMeasurement(minimaxlink.OfficialOutputSeconds, duration) && duration == math.Trunc(duration) {
		stored["duration"] = duration
	}
	fields := map[string]string{
		minimaxlink.OfficialOutputSeconds:     "output_seconds",
		minimaxlink.OfficialInputVideoSeconds: "input_seconds",
		minimaxlink.OfficialInputImageCount:   "input_image_count",
	}
	measurements := make(map[string]float64, len(fields))
	usage, _ := raw.Task["usage"].(map[string]any)
	for field, wireField := range fields {
		if amount, ok := usage[wireField].(float64); ok && minimaxlink.ValidOfficialMeasurement(field, amount) {
			measurements[field] = amount
		}
	}
	if async := task.PrivateData.AsyncBilling; async != nil && async.TieredSnapshot != nil && reportedResolution != "" {
		expected, _ := async.TieredSnapshot.UsageFacts["resolution"].(string)
		if !strings.EqualFold(reportedResolution, expected) {
			return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "MiniMax official output resolution differs from the frozen request"}
		}
	}
	if len(measurements) > 0 {
		stored["official_usage"] = measurements
	}
	tokenUsage := thirdparty.NormalizeTerminalTokenUsage(map[string]any{"task": map[string]any{"usage": usage}})
	if len(tokenUsage.Usage) > 0 {
		stored["usage"] = tokenUsage.Usage
		stored["usage_source"] = tokenUsage.Source
	}
	if len(tokenUsage.Evidence) > 0 {
		stored["usage_evidence"] = tokenUsage.Evidence
	}
	normalized, err = common.Marshal(stored)
	return observation, normalized, err
}
