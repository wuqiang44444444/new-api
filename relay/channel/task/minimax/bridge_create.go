package minimax

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Hook admission timeouts bound the wait for an engine slot. A timeout on the
// create path rejects the request before any funds are held; a timeout at
// poll time is an observation failure, never a task failure or refund.
const (
	createAdmissionTimeout = 2 * time.Second
	pollAdmissionTimeout   = 2 * time.Second
)

// minimaxCreateConversion caches the once-validated plugin conversion so the
// pre-pricing validation and the request body derive from one deterministic
// buildCreate result.
type minimaxCreateConversion struct {
	body   []byte
	plugin *pluginruntime.LoadedPlugin
}

// ensureCreateConversion runs the artifact's buildCreate hook once per
// submission and validates the result against host-owned invariants before
// it can reach pricing or the provider. It runs before pricing and before
// the durable attempt, so declared-scope violations return HTTP 400
// invalid_request without an attempt, a funds hold or a provider call.
func (a *TaskAdaptor) ensureCreateConversion(requestContext *gin.Context, info *relaycommon.RelayInfo) (*minimaxCreateConversion, error) {
	if a.pluginCreate != nil {
		return a.pluginCreate, nil
	}
	plugin := PinnedMinimaxExtension(requestContext)
	if plugin == nil {
		return nil, fmt.Errorf("the minimax-link extension plugin is unavailable for this request")
	}
	configuration, err := pinnedConfiguration(requestContext)
	if err != nil {
		return nil, err
	}
	contract, ok := relaycommon.GetVideoContractRequest(requestContext)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, fmt.Errorf("the minimax-link extension requires a ModelArk V3 request contract")
	}
	upstreamModel := providerModelFromRelayInfo(info, contract.ModelArk.Model)
	if info != nil && info.ChannelMeta != nil && !info.IsModelMapped && strings.TrimSpace(info.UpstreamModelName) == "" {
		// Mirror the typed-channel write-back so the frozen task snapshot
		// keeps the resolved provider model.
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
		"protocol":      protocolName(),
		"providerModel": upstreamModel,
		"request":       requestMap,
		"limits":        map[string]any{"maxDurationSeconds": relaycommon.MaxTaskDurationSeconds},
	}
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		requestContextFrom(requestContext), createAdmissionTimeout,
		hookRoot, []string{protocolName(), "buildCreate"}, input,
	)
	if callErr != nil {
		return nil, relaycommon.NewVideoContractError("invalid_request", hookErrorMessage(callErr))
	}
	conversion, decodeErr := decodeCreateConversion(result, configuration, upstreamModel, contract.ModelArk)
	if decodeErr != nil {
		return nil, decodeErr
	}
	conversion.plugin = plugin
	a.pluginCreate = conversion
	return conversion, nil
}

// decodeCreateConversion validates the plugin conversion against host-owned
// invariants. The plugin owns the JD conversion; the host verifies that the
// conversion preserved the northbound contract: provider-model identity, the
// duration billing multiplier, full content preservation, the fixed southbound
// policy (prompt optimization on, watermark off), enum scope, and that no
// undeclared fields or plugin billing inputs appeared.
func decodeCreateConversion(
	result any,
	configuration *pluginruntime.SeedanceChannelConfiguration,
	providerModel string,
	contract *taskdto.ModelArkVideoCreateRequest,
) (*minimaxCreateConversion, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("minimax plugin buildCreate returned an invalid result")
	}
	for _, forbidden := range []string{"probe", "createPath", "queryPath"} {
		if _, exists := object[forbidden]; exists {
			return nil, fmt.Errorf("minimax plugin buildCreate returned a host-owned field %q", forbidden)
		}
	}
	body, ok := object["body"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("minimax plugin buildCreate returned an invalid body")
	}
	if len(body) != 3 {
		return nil, fmt.Errorf("minimax plugin body must contain exactly model, content and parameters")
	}
	wireModel, err := pluginStringField(body, "model", 191)
	if err != nil {
		return nil, err
	}
	if wireModel != providerModel {
		return nil, fmt.Errorf("minimax plugin body model does not match the resolved provider model")
	}
	if err := validateWireContent(body["content"], contract); err != nil {
		return nil, err
	}
	parameters, ok := body["parameters"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("minimax plugin body must carry a parameters object")
	}
	if len(parameters) != 5 {
		return nil, fmt.Errorf("minimax plugin parameters must contain exactly duration, resolution, ratio, prompt_optimizer and watermark")
	}
	metadata, err := declaredMetadata(configuration, providerModel)
	if err != nil {
		return nil, err
	}
	duration, err := pluginDurationField(parameters)
	if err != nil {
		return nil, err
	}
	effective, err := effectiveRequestDuration(contract, metadata)
	if err != nil {
		return nil, err
	}
	if duration != effective {
		return nil, failInvalidParameter("minimax plugin body duration does not match the request duration")
	}
	resolution, err := pluginStringField(parameters, "resolution", 64)
	if err != nil {
		return nil, err
	}
	if !enumValuePreserved(metadata, contract.Resolution, resolution, func(m *pluginruntime.SeedanceVideoModelMetadata) []string {
		return m.Resolutions
	}) {
		return nil, fmt.Errorf("minimax plugin body resolution does not match the request or the declared open set")
	}
	ratio, err := pluginStringField(parameters, "ratio", 64)
	if err != nil {
		return nil, err
	}
	if !enumValuePreserved(metadata, contract.Ratio, ratio, func(m *pluginruntime.SeedanceVideoModelMetadata) []string {
		return m.Ratios
	}) {
		return nil, fmt.Errorf("minimax plugin body ratio does not match the request or the declared open set")
	}
	if optimizer, exists := parameters["prompt_optimizer"]; !exists || optimizer != true {
		return nil, fmt.Errorf("minimax plugin must keep prompt optimization as the fixed southbound policy")
	}
	if watermark, exists := parameters["watermark"]; !exists || watermark != false {
		return nil, fmt.Errorf("minimax plugin must keep watermark off as the fixed southbound policy")
	}
	wireBytes, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &minimaxCreateConversion{body: wireBytes, plugin: nil}, nil
}
