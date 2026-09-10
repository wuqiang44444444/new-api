package seedance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// seedanceFeicaiCreateWireRequest mirrors the legacy wire shape exactly
// (field order and omitempty), so the re-marshaled plugin body is
// byte-identical to the legacy Go conversion output.
type seedanceFeicaiCreateWireRequest struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	Duration int      `json:"duration"`
	Ratio    string   `json:"ratio"`
	Images   []string `json:"images,omitempty"`
	Audios   []string `json:"audios,omitempty"`
	Videos   []string `json:"videos,omitempty"`
}

// seedancePluginCreateConversion is the validated result of the plugin's
// buildCreate hook, cached on the adaptor for the whole submission (probe
// and request body derive from the same conversion).
type seedancePluginCreateConversion struct {
	plugin *pluginruntime.LoadedPlugin
	body   []byte
	probe  map[string]any
}

// seedanceHookError maps engine hook failures to plain errors carrying the
// sanitized hook message (the legacy message strings are thrown verbatim by
// the plugin). The full engine error — with hook name and stack — stays in
// logs only, never in client-visible errors.
func seedanceHookError(err error) error {
	var hookErr *pluginruntime.HookError
	if errors.As(err, &hookErr) {
		return errors.New(hookErr.Message)
	}
	return err
}

func pinnedSeedanceExtension(c *gin.Context) *pluginruntime.LoadedPlugin {
	value, exists := c.Get(pluginruntime.ContextKeyPinnedPlugin)
	if !exists {
		return nil
	}
	pinned, ok := value.(pluginruntime.PinnedPlugin)
	if !ok || pinned.Plugin == nil {
		return nil
	}
	if pinned.Plugin.Meta.Key != SeedanceExtensionPluginKey {
		return nil
	}
	return pinned.Plugin
}

// ensureSeedanceCreateConversion runs the plugin's buildCreate hook once per
// submission and caches the validated conversion on the adaptor. It is
// called before pricing/hold (billing probe) and again for the request body
// after the attempt barrier; both consumers share one deterministic result.
func (a *TaskAdaptor) ensureSeedanceCreateConversion(c *gin.Context, info *relaycommon.RelayInfo) (*seedancePluginCreateConversion, error) {
	if a.pluginCreate != nil {
		return a.pluginCreate, nil
	}
	if !SeedanceExtensionProtocolMigrated(a.protocol) {
		return nil, fmt.Errorf("the selected video adapter requires the Seedance extension")
	}
	plugin := pinnedSeedanceExtension(c)
	if plugin == nil {
		return nil, fmt.Errorf("the seedance-link extension plugin is unavailable for this request")
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, fmt.Errorf("the selected video adapter requires a ModelArk request")
	}
	upstreamModel := providerModelFromRelayInfo(info, contract.ModelArk.Model)
	if info != nil && info.ChannelMeta != nil && !info.IsModelMapped && strings.TrimSpace(info.UpstreamModelName) == "" {
		// Mirror the legacy capability write-back so the frozen task
		// upstream model snapshot keeps the resolved provider model.
		info.UpstreamModelName = upstreamModel
	}
	requestBytes, err := common.Marshal(contract.ModelArk)
	if err != nil {
		return nil, err
	}
	var requestMap map[string]any
	if err = common.Unmarshal(requestBytes, &requestMap); err != nil {
		return nil, err
	}
	input := map[string]any{
		"protocol":      string(a.protocol),
		"providerModel": upstreamModel,
		"request":       requestMap,
		"limits":        map[string]any{"maxDurationSeconds": relaycommon.MaxTaskDurationSeconds},
	}
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		seedancePluginRequestContext(c), seedanceExtensionCreateAdmissionTimeout,
		"seedance", []string{string(a.protocol), "buildCreate"}, input,
	)
	if callErr != nil {
		return nil, seedanceHookError(callErr)
	}
	conversion, decodeErr := decodeSeedanceCreateConversion(result, a.protocol, upstreamModel, contract.ModelArk.Duration)
	if decodeErr != nil {
		return nil, decodeErr
	}
	conversion.plugin = plugin
	a.pluginCreate = conversion
	return conversion, nil
}

// decodeSeedanceCreateConversion validates the plugin conversion against
// host-owned invariants before it can reach pricing or the provider: the
// wire model must equal the resolved provider model, the wire duration must
// equal the request duration (a billing multiplier the plugin may not
// rewrite), and probe keys are restricted to the per-protocol whitelist.
func decodeSeedanceCreateConversion(result any, protocol dto.VideoUpstreamProtocol, expectedModel string, expectedDuration *int) (*seedancePluginCreateConversion, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("seedance plugin buildCreate returned an invalid result")
	}
	body, ok := object["body"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("seedance plugin buildCreate returned an invalid body")
	}
	wire := seedanceFeicaiCreateWireRequest{}
	model, err := seedancePluginStringField(body, "model", 191)
	if err != nil {
		return nil, err
	}
	wire.Model = model
	prompt, err := seedancePluginStringField(body, "prompt", 1<<20)
	if err != nil {
		return nil, err
	}
	wire.Prompt = prompt
	duration, err := seedancePluginDurationField(body, "duration")
	if err != nil {
		return nil, err
	}
	wire.Duration = duration
	ratio, err := seedancePluginStringField(body, "ratio", 64)
	if err != nil {
		return nil, err
	}
	wire.Ratio = ratio
	for field, target := range map[string]*[]string{"images": &wire.Images, "audios": &wire.Audios, "videos": &wire.Videos} {
		values, err := seedancePluginStringSliceField(body, field, 64)
		if err != nil {
			return nil, err
		}
		*target = values
	}
	wireBytes, err := common.Marshal(wire)
	if err != nil {
		return nil, err
	}
	probe, err := decodeSeedanceProbeExtension(object["probe"], protocol)
	if err != nil {
		return nil, err
	}
	// Host-owned identity and billing-multiplier invariants: the plugin
	// transforms the wire shape, it may not rewrite who is called or the
	// duration the request was priced against.
	if wire.Model != expectedModel {
		return nil, fmt.Errorf("seedance plugin body model does not match the resolved provider model")
	}
	if expectedDuration == nil || wire.Duration != *expectedDuration {
		return nil, fmt.Errorf("seedance plugin body duration does not match the request duration")
	}
	return &seedancePluginCreateConversion{body: wireBytes, probe: probe}, nil
}

// decodeSeedanceProbeExtension accepts only whitelisted probe keys for the
// protocol. Host-derived billing inputs (duration_seconds, has_video_input,
// generate_audio, input_mode, control_mode) are never whitelisted, so a
// plugin — embedded or uploaded — cannot rewrite them.
func decodeSeedanceProbeExtension(value any, protocol dto.VideoUpstreamProtocol) (map[string]any, error) {
	allowed := seedanceExtensionProbeFields[protocol]
	if allowed == nil {
		return nil, fmt.Errorf("seedance plugin probe fields are not registered for protocol %s", protocol)
	}
	probeObject, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("seedance plugin buildCreate returned an invalid probe")
	}
	probe := make(map[string]any, len(probeObject))
	for key, fieldValue := range probeObject {
		if _, permitted := allowed[key]; !permitted {
			return nil, fmt.Errorf("seedance plugin probe field %q is not permitted for protocol %s", key, protocol)
		}
		if len(key) > 64 {
			return nil, fmt.Errorf("seedance plugin probe key %q is invalid", key)
		}
		switch typed := fieldValue.(type) {
		case string:
			if len(typed) > 64 {
				return nil, fmt.Errorf("seedance plugin probe field %q is invalid", key)
			}
			probe[key] = typed
		case bool:
			probe[key] = typed
		default:
			number, ok := seedancePluginNumberField(fieldValue)
			if !ok {
				return nil, fmt.Errorf("seedance plugin probe field %q has an unsupported value", key)
			}
			if math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 || number > 1e9 {
				return nil, fmt.Errorf("seedance plugin probe field %q is invalid", key)
			}
			probe[key] = number
		}
	}
	if len(probe) == 0 {
		return nil, fmt.Errorf("seedance plugin buildCreate returned an empty probe")
	}
	return probe, nil
}

func seedancePluginStringField(object map[string]any, name string, maxBytes int) (string, error) {
	value, exists := object[name]
	if !exists || value == nil {
		return "", fmt.Errorf("seedance plugin field %q is required", name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("seedance plugin field %q must be a string", name)
	}
	if text == "" || len(text) > maxBytes {
		return "", fmt.Errorf("seedance plugin field %q is invalid", name)
	}
	return text, nil
}

// seedancePluginNumberField normalizes a numeric hook output. Sobek exports
// JS integers as int64 and JS floats as float64.
func seedancePluginNumberField(value any) (float64, bool) {
	switch number := value.(type) {
	case int64:
		return float64(number), true
	case int:
		return float64(number), true
	case float64:
		return number, true
	default:
		return 0, false
	}
}

// seedancePluginDurationField re-validates the billing multiplier bound in
// Go: the plugin enforces per-model ranges, but the host independently caps
// duration before it can reach quota calculation.
func seedancePluginDurationField(object map[string]any, name string) (int, error) {
	value, exists := object[name]
	if !exists || value == nil {
		return 0, fmt.Errorf("seedance plugin field %q is required", name)
	}
	number, ok := seedancePluginNumberField(value)
	if !ok || number != math.Trunc(number) {
		return 0, fmt.Errorf("seedance plugin field %q must be an integer", name)
	}
	if number <= 0 || number > relaycommon.MaxTaskDurationSeconds {
		return 0, fmt.Errorf("seedance plugin field %q exceeds the host duration bound", name)
	}
	return int(number), nil
}

func seedancePluginStringSliceField(object map[string]any, name string, maxItems int) ([]string, error) {
	value, exists := object[name]
	if !exists || value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("seedance plugin field %q must be an array", name)
	}
	if len(items) > maxItems {
		return nil, fmt.Errorf("seedance plugin field %q has too many items", name)
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || text == "" || len(text) > 20*1024*1024 {
			return nil, fmt.Errorf("seedance plugin field %q has an invalid item", name)
		}
		values = append(values, text)
	}
	return values, nil
}

// buildSeedancePluginCreateRequestBody serves the once-validated conversion.
// The prepared transaction already protects the frozen version until its
// execution references are released; HTTP needs no repeated existence query.
func (a *TaskAdaptor) buildSeedancePluginCreateRequestBody(c *gin.Context, info *relaycommon.RelayInfo, profile dto.VideoUpstreamProfile) ([]byte, bool, error) {
	if profile != dto.VideoUpstreamProfileThirdPartyFeicaiVideos || !SeedanceExtensionProtocolMigrated(a.protocol) {
		return nil, false, nil
	}
	conversion, err := a.ensureSeedanceCreateConversion(c, info)
	if err != nil {
		return nil, true, err
	}
	return conversion.body, true, nil
}

// normalizeSeedanceVideoCreateResponse routes create-response normalization:
// submissions converted by the plugin parse through the plugin hook; every
// other profile keeps the legacy Go normalization.
func normalizeSeedanceVideoCreateResponse(c *gin.Context, a *TaskAdaptor, body []byte) ([]byte, error) {
	if a.pluginCreate != nil && a.profile == dto.VideoUpstreamProfileThirdPartyFeicaiVideos {
		return parseSeedancePluginCreateResponse(c, a, body)
	}
	return normalizeVideoCreateResponse(a.profile, body)
}

func parseSeedancePluginCreateResponse(c *gin.Context, a *TaskAdaptor, body []byte) ([]byte, error) {
	plugin := a.pluginCreate.plugin
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		seedancePluginRequestContext(c), seedanceExtensionCreateAdmissionTimeout,
		"seedance", []string{string(a.protocol), "parseCreateResponse"},
		map[string]any{"body": string(body)},
	)
	if callErr != nil {
		return nil, seedanceHookError(callErr)
	}
	object, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("seedance plugin parseCreateResponse returned an invalid result")
	}
	id, err := seedancePluginStringField(object, "id", 1<<16)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || len(trimmed) > 191 || hasControlRunes(trimmed) {
		return nil, fmt.Errorf("upstream create response has an invalid id")
	}
	return common.Marshal(map[string]any{"id": trimmed})
}

func hasControlRunes(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// normalizeSeedanceVideoTaskResponse routes polling normalization by the
// frozen plugin snapshot: tasks created after migration resolve their exact
// plugin version; historical tasks without a snapshot keep the Go path.
func normalizeSeedanceVideoTaskResponse(
	ctx context.Context,
	task *model.Task,
	profile dto.VideoUpstreamProfile,
	adapterVersion relaycommon.VideoSouthboundAdapterVersion,
	body []byte,
	expectedTaskID string,
	baseURL string,
	billingContext *relaycommon.VideoTaskBillingContext,
) ([]byte, error) {
	snapshot := seedancePluginTaskSnapshot(task)
	if snapshot == nil || snapshot.Key != SeedanceExtensionPluginKey || snapshot.Version == "" {
		return normalizeVideoTaskResponse(profile, adapterVersion, body, expectedTaskID, baseURL, billingContext)
	}
	plugin, err := seedanceExtensions.ResolveVersion(ctx, snapshot.Version)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{
			Reason: fmt.Sprintf("seedance plugin version %q is unavailable", snapshot.Version),
		}
	}
	protocol := task.PrivateData.VideoUpstreamProtocol
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		ctx, seedanceExtensionPollAdmissionTimeout,
		"seedance", []string{string(protocol), "parseTaskObservation"},
		map[string]any{"taskId": expectedTaskID, "body": string(body)},
	)
	if callErr != nil {
		return nil, seedancePollObservationError(callErr)
	}
	return decodeSeedanceTaskObservation(result, expectedTaskID, baseURL)
}

// seedancePollObservationError classifies plugin hook failures at poll time:
// a JavaScript exception (HookError) is a plugin contract defect and parks
// the task in reconciliation with the sanitized message; every other engine
// outcome — admission timeout, execution timeout, interrupt — is a local
// infrastructure condition, not provider truth. Those wrap the shared
// sentinel so the poller skips the round without counting toward the
// failure cutoff and without touching task or billing state.
func seedancePollObservationError(err error) error {
	var hookErr *pluginruntime.HookError
	if errors.As(err, &hookErr) {
		return &relaycommon.UpstreamContractViolation{Reason: hookErr.Message}
	}
	return fmt.Errorf("%w: %v", relaycommon.ErrUpstreamObservationUnavailable, err)
}

func decodeSeedanceTaskObservation(result any, expectedTaskID, baseURL string) ([]byte, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin observation result"}
	}
	if violation, exists := object["violation"]; exists {
		reason, ok := violation.(string)
		if !ok || reason == "" || len(reason) > 256 {
			reason = "invalid plugin observation result"
		}
		return nil, &relaycommon.UpstreamContractViolation{Reason: reason}
	}
	id, err := seedancePluginStringField(object, "id", 1<<16)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin observation id"}
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != expectedTaskID {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "task id mismatch"}
	}
	status, err := seedancePluginStringField(object, "status", 32)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin observation status"}
	}
	normalized := map[string]any{"id": strings.TrimSpace(id)}
	switch status {
	case "queued":
		normalized["status"] = "queued"
	case "running":
		normalized["status"] = "running"
	case "succeeded":
		videoURL, _ := object["videoUrl"].(string)
		if len(videoURL) > 20*1024*1024 {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid completed video url"}
		}
		validated, urlErr := relaycommon.ValidateSameOriginVideoResultURL(videoURL, baseURL)
		if urlErr != nil {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid completed video url"}
		}
		normalized["status"] = "succeeded"
		normalized["content"] = map[string]any{"video_url": validated}
	case "failed":
		errorObject, ok := object["error"].(map[string]any)
		if !ok {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid plugin failure detail"}
		}
		code, _ := errorObject["code"].(string)
		message, _ := errorObject["message"].(string)
		normalized["status"] = "failed"
		normalized["error"] = map[string]any{
			"code":    sanitizeSeedanceTaskError(code, 64),
			"message": sanitizeSeedanceTaskError(message, 500),
		}
	default:
		return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported task status"}
	}
	return common.Marshal(normalized)
}

// sanitizeSeedanceTaskError mirrors the legacy failure-detail sanitization:
// public-safe message, rune limit, control runes replaced by spaces.
func sanitizeSeedanceTaskError(value string, limit int) string {
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

// seedancePluginRequestContext returns the request context for plugin hook
// calls, tolerating synthetic gin contexts without a request.
func seedancePluginRequestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}

// seedancePluginTaskSnapshot returns the frozen plugin identity of a task,
// nil for historical tasks created before the protocol migrated.
func seedancePluginTaskSnapshot(task *model.Task) *model.TaskPluginSnapshot {
	if task == nil || task.PrivateData.Execution == nil {
		return nil
	}
	return task.PrivateData.Execution.TaskPlugin
}
