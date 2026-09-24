package minimax

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// decodeCreateResponse validates the artifact's parseCreateResponse output:
// a trusted upstream task id with the registered acceptance state.
func decodeCreateResponse(result any) (string, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return "", errors.New("minimax plugin parseCreateResponse returned an invalid result")
	}
	providerID, err := pluginStringField(object, "id", 1<<16)
	if err != nil {
		return "", err
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || len(providerID) > 191 || hasControlRunes(providerID) {
		return "", errors.New("upstream create response has an invalid id")
	}
	if status, err := pluginStringField(object, "status", 32); err != nil || status != acceptedCreateStatus {
		return "", errors.New("upstream create response status is not a registered acceptance state")
	}
	return providerID, nil
}

func hasControlRunes(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// normalizedObservation carries the host-validated parts of a task
// observation. VideoURL and CreditEvidence stay in memory: signed URLs and
// credit values never persist into the task row.
type normalizedObservation struct {
	ID             string
	Status         string
	VideoURL       string
	FailureCode    string
	FailureReason  string
	CreditEvidence map[string]int
	CreditSource   string
}

// normalizeTaskObservation resolves the exact frozen artifact version, runs
// its parseTaskObservation hook on the raw upstream observation, and
// validates the result. The plugin performs the JD shape conversion; the
// host verifies task identity, the status domain, the result URL shape and
// the usage locator before anything is persisted or delivered.
func normalizeTaskObservation(ctx context.Context, task *model.Task, rawBody []byte, expectedTaskID string) (*normalizedObservation, []byte, error) {
	snapshot := taskPluginSnapshot(task)
	if snapshot == nil || snapshot.Key != pluginruntime.MinimaxPluginKey || snapshot.Version == "" {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "task has no frozen minimax-link plugin snapshot"}
	}
	plugin, err := minimaxplugin.Default.ResolveVersion(ctx, snapshot.Version)
	if err != nil {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: fmt.Sprintf("minimax-link plugin version %q is unavailable", snapshot.Version)}
	}
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		ctx, pollAdmissionTimeout,
		hookRoot, []string{protocolName(), "parseTaskObservation"},
		map[string]any{"taskId": expectedTaskID, "body": string(rawBody)},
	)
	if callErr != nil {
		return nil, nil, pollObservationError(callErr)
	}
	return decodeTaskObservation(result, rawBody, expectedTaskID)
}

// Observation status domain after artifact normalization. These are the
// internal typed-video observation states, not JD spellings.
const (
	observationStatusQueued    = "queued"
	observationStatusRunning   = "running"
	observationStatusSucceeded = "succeeded"
	observationStatusFailed    = "failed"
	observationStatusCancelled = "cancelled"
)

// acceptedCreateStatus is the only registered create acceptance state.
// Evidence-backed states may extend it; the upstream query status enum never
// widens the create contract.
const acceptedCreateStatus = "pending"

// pollObservationError classifies hook failures at poll time: a JavaScript
// exception is a plugin contract defect and parks the task in reconciliation;
// engine timeouts and interrupts are local infrastructure conditions that
// skip the round without touching task or billing state.
func pollObservationError(err error) error {
	var hookErr *pluginruntime.HookError
	if errors.As(err, &hookErr) {
		return &relaycommon.UpstreamContractViolation{Reason: hookErr.Message}
	}
	return fmt.Errorf("%w: %v", relaycommon.ErrUpstreamObservationUnavailable, err)
}

func taskPluginSnapshot(task *model.Task) *model.TaskPluginSnapshot {
	if task == nil || task.PrivateData.Execution == nil {
		return nil
	}
	return task.PrivateData.Execution.TaskPlugin
}

// decodeTaskObservation validates the artifact observation against host-owned
// invariants and derives the stored body plus the in-memory delivery facts.
// Stored bodies carry only the normalized status, a sanitized failure detail
// and the usage locator fields: signed URLs and raw responses never persist
// into the task row.
func decodeTaskObservation(result any, rawBody []byte, expectedTaskID string) (*normalizedObservation, []byte, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin observation result"}
	}
	if violation, exists := object["violation"]; exists {
		reason, _ := violation.(string)
		if reason == "" || len(reason) > 256 {
			reason = "invalid plugin observation result"
		}
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: reason}
	}
	observationID, err := pluginStringField(object, "id", 1<<16)
	if err != nil {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin observation id"}
	}
	observationID = strings.TrimSpace(observationID)
	if observationID == "" || observationID != expectedTaskID {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "task id mismatch"}
	}
	status, err := pluginStringField(object, "status", 32)
	if err != nil {
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin observation status"}
	}
	observation := &normalizedObservation{ID: observationID, Status: status}
	stored := map[string]any{"id": observationID, "status": status}
	switch status {
	case observationStatusQueued, observationStatusRunning, observationStatusCancelled:
		// non-terminal accepted states
	case observationStatusSucceeded:
		videoURL, err := validatedObservationVideoURL(object)
		if err != nil {
			return nil, nil, err
		}
		observation.VideoURL = videoURL
		scanRoot, err := observationUsageScanRoot(object)
		if err != nil {
			return nil, nil, err
		}
		if scanRoot != "" {
			// The host reads the located subtree from the raw observation and
			// validates the credit value; the artifact never supplies the
			// number itself. The locator and evidence travel inside the stored
			// body so the shared redaction strips them before persistence,
			// mirroring the usage-evidence fields of the other typed video
			// protocols.
			observation.CreditEvidence, observation.CreditSource = NormalizeCreditUsage(rawBody, scanRoot)
			if observation.CreditEvidence != nil {
				stored["usage_source"] = observation.CreditSource
				stored["usage_evidence"] = observation.CreditEvidence
			}
		}
	case observationStatusFailed:
		code, reason, err := validatedFailureDetail(object)
		if err != nil {
			return nil, nil, err
		}
		observation.FailureCode = code
		observation.FailureReason = reason
		stored["error"] = map[string]any{"code": code, "message": reason}
	default:
		return nil, nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported task status"}
	}
	storedBytes, err := common.Marshal(stored)
	if err != nil {
		return nil, nil, err
	}
	return observation, storedBytes, nil
}

// validatedObservationVideoURL validates the located success video URL. JD
// result URLs live on CDN origins unrelated to the API base URL, so the host
// validates the URL shape instead of a same-origin rule; SSRF validation
// still applies at fetch time and no credential ever attaches to it.
func validatedObservationVideoURL(object map[string]any) (string, error) {
	value, exists := object["videoUrl"]
	if !exists {
		return "", &relaycommon.UpstreamContractViolation{Reason: "success observation has no video result"}
	}
	videoURL, ok := value.(string)
	if !ok || videoURL == "" || len(videoURL) > 2048 {
		return "", &relaycommon.UpstreamContractViolation{Reason: "invalid completed video url"}
	}
	parsed, err := url.Parse(videoURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", &relaycommon.UpstreamContractViolation{Reason: "invalid completed video url"}
	}
	return videoURL, nil
}

// validatedFailureDetail validates and sanitizes the located failure detail.
func validatedFailureDetail(object map[string]any) (string, string, error) {
	value, exists := object["error"]
	if !exists {
		return "", "", &relaycommon.UpstreamContractViolation{Reason: "failure observation has no trusted error detail"}
	}
	detail, ok := value.(map[string]any)
	if !ok {
		return "", "", &relaycommon.UpstreamContractViolation{Reason: "invalid plugin failure detail"}
	}
	code, _ := detail["code"].(string)
	message, _ := detail["message"].(string)
	return sanitizeObservationError(code, 64), sanitizeObservationError(message, 500), nil
}

// sanitizeObservationError mirrors the shared failure-detail sanitization:
// public-safe message, rune limit, control runes replaced by spaces.
func sanitizeObservationError(value string, limit int) string {
	runes := []rune(common.PublicTaskErrorMessage(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	for i, r := range runes {
		if unicode.IsControl(r) {
			runes[i] = ' '
		}
	}
	return strings.TrimSpace(string(runes))
}

// observationUsageScanRoot validates the usage locator emitted by the
// artifact. Exactly one locator is accepted for this protocol.
func observationUsageScanRoot(object map[string]any) (string, error) {
	value, exists := object["usageScanRoot"]
	if !exists {
		return "", nil
	}
	scanRoot, ok := value.(string)
	if !ok || len(scanRoot) > 128 {
		return "", &relaycommon.UpstreamContractViolation{Reason: "invalid plugin usage locator"}
	}
	if _, registered := creditUsageScanRoots[scanRoot]; !registered {
		return "", &relaycommon.UpstreamContractViolation{Reason: "unsupported plugin usage locator"}
	}
	return scanRoot, nil
}
