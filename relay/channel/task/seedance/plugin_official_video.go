package seedance

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// The plugin controls Provider conversion; identity and charge multipliers
// remain independently checked before hold and before any request is sent.
func decodeOfficialPluginCreate(body map[string]any, probe any, model string, duration *int) (*seedancePluginCreateConversion, error) {
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	var wire requestPayload
	if err = common.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("invalid ModelArk plugin body")
	}
	if wire.Model != model {
		return nil, fmt.Errorf("seedance plugin body model does not match the resolved provider model")
	}
	var actual *int
	if wire.Duration != nil {
		value := int(*wire.Duration)
		actual = &value
	}
	if !reflect.DeepEqual(actual, duration) {
		return nil, fmt.Errorf("seedance plugin body duration does not match the request duration")
	}
	fields, ok := probe.(map[string]any)
	if !ok || len(fields) != 0 {
		return nil, fmt.Errorf("official ModelArk plugin cannot replace host billing inputs")
	}
	// Re-marshal through the published request type, preserving optional scalar
	// zero/false values and the original field ordering.
	data, err = common.Marshal(wire)
	return &seedancePluginCreateConversion{body: data, probe: map[string]any{}}, err
}

// decodeOfficialPluginObservation decodes a non-feicai plugin observation.
// apiVersion selects the observation contract: artifacts at or above the usage
// scan version only locate the Provider usage subtree and the host derives
// usage; frozen v2 artifacts keep their embedded-usage contract with host-side
// bounds validation.
func decodeOfficialPluginObservation(result any, expectedID string, apiVersion int, protocol dto.VideoUpstreamProtocol) ([]byte, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk plugin observation"}
	}
	if violation, exists := object["violation"]; exists {
		reason, ok := violation.(string)
		if !ok || len(reason) > 256 || reason == "" {
			reason = "invalid ModelArk plugin observation"
		}
		return nil, &relaycommon.UpstreamContractViolation{Reason: reason}
	}
	body, ok := object["body"].(string)
	if !ok {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk plugin observation"}
	}
	if apiVersion >= pluginruntime.SeedanceUsageScanAPIVersion && !seedanceOfficialUsageProtocol(protocol) {
		return decodeUsageScanPluginObservation(body, expectedID)
	}
	var response responseTask
	if err := common.Unmarshal([]byte(body), &response); err != nil || response.ID == "" || response.ID != expectedID {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "ModelArk task id mismatch"}
	}
	// Final charging is host-owned. Even a modified artifact cannot supply
	// negative or overflowing usage as accepted Provider evidence.
	if response.Usage != nil {
		for _, count := range []*int{response.Usage.CompletionTokens, response.Usage.TotalTokens} {
			if count != nil && (*count < 0 || int64(*count) > 2147483647) {
				return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk plugin usage"}
			}
		}
	}
	for _, count := range response.UsageEvidence {
		if count < 0 || int64(count) > 2147483647 {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk plugin usage evidence"}
		}
	}
	return []byte(body), nil
}

// seedanceOfficialUsageProtocol reports whether a protocol bills through the
// official ModelArk strict-integer usage semantics. For these protocols the
// host derives usage from the raw upstream bytes before the artifact sees the
// body: the strict integer lexeme rule cannot survive the JS number boundary.
func seedanceOfficialUsageProtocol(protocol dto.VideoUpstreamProtocol) bool {
	return protocol == dto.VideoUpstreamProtocolModelArkV3Volcengine ||
		protocol == dto.VideoUpstreamProtocolModelArkV3BytePlus ||
		protocol == dto.VideoUpstreamProtocolModelArkV3CMCC
}

// seedanceUsageScanBodyLimit bounds the serialized observation body of a v3
// artifact: the scan root duplicates a subtree of the provider payload, so the
// result stays within the same order of magnitude as the input body.
const seedanceUsageScanBodyLimit = 1 << 20

// decodeUsageScanPluginObservation applies the host-owned generic usage
// derivation to a v3 artifact body: the artifact only locates the Provider
// usage subtree; the host buckets candidates, blocks on invalid fields and
// forms usage, source and evidence with thirdparty.NormalizeTerminalTokenUsage.
// Plugin-supplied usage fields are stripped first, so a modified artifact
// cannot supply charging facts, and a succeeded task without a scan root is a
// contract violation.
func decodeUsageScanPluginObservation(body string, expectedID string) ([]byte, error) {
	if body == "" || len(body) > seedanceUsageScanBodyLimit {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk plugin observation"}
	}
	var root map[string]any
	if err := common.Unmarshal([]byte(body), &root); err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid ModelArk plugin observation"}
	}
	id, _ := root["id"].(string)
	if strings.TrimSpace(id) == "" || id != expectedID {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "ModelArk task id mismatch"}
	}
	status, _ := root["status"].(string)
	scan, hasScan := root["usage_scan_root"].(map[string]any)
	delete(root, "usage")
	delete(root, "usage_source")
	delete(root, "usage_evidence")
	delete(root, "usage_scan_root")
	if status == "succeeded" && !hasScan {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "plugin usage scan root is missing"}
	}
	if hasScan {
		usage := thirdparty.NormalizeTerminalTokenUsage(scan)
		if usage.Usage != nil {
			root["usage"] = usage.Usage
			root["usage_source"] = usage.Source
		}
		if len(usage.Evidence) > 0 {
			root["usage_evidence"] = usage.Evidence
		}
	}
	return common.Marshal(root)
}

// Plugin tasks use their frozen path; pre-plugin history retains its original
// resolver. No active definition or current Channel is consulted for polling.
func seedanceTaskPath(task *model.Task, profile dto.VideoUpstreamProfile, taskID, baseURL string) (string, error) {
	if seedancePluginTaskSnapshot(task) != nil && task.PrivateData.VideoUpstreamQueryPathTemplate != "" {
		template := task.PrivateData.VideoUpstreamQueryPathTemplate
		if err := dto.ValidateVideoUpstreamURL(baseURL, "/create", template); err != nil {
			return "", err
		}
		return strings.Replace(template, "{task_id}", url.PathEscape(taskID), 1), nil
	}
	return videoTaskPath(profile, task.PrivateData.VideoUpstreamQueryPathTemplate, taskID)
}

// Official ModelArk uses the published request shape unchanged except for the
// mapped model. Compare the complete body before hold so frames, audio, media
// inputs and resolution cannot diverge from the request priced by the host.
func validateOfficialPluginBillingRequest(body any, request map[string]any, providerModel string) error {
	expected := make(map[string]any, len(request))
	for key, value := range request {
		expected[key] = value
	}
	expected["model"] = providerModel
	actualBytes, err := common.Marshal(body)
	if err != nil {
		return err
	}
	expectedBytes, err := common.Marshal(expected)
	if err != nil {
		return err
	}
	var actual, normalizedExpected map[string]any
	if err := common.Unmarshal(actualBytes, &actual); err != nil {
		return err
	}
	if err := common.Unmarshal(expectedBytes, &normalizedExpected); err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, normalizedExpected) {
		return fmt.Errorf("official plugin body differs from the request priced by the host")
	}
	return nil
}
