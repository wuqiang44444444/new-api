package seedance

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// prepareDraft gates wire shape, not provider entitlements. Drafts using a
// ModelArk-shaped transport are submitted to the selected provider unchanged.
func (a *TaskAdaptor) prepareDraft(c *gin.Context, info *relaycommon.RelayInfo, request *taskdto.ModelArkVideoCreateRequest) *taskdto.TaskError {
	requested := request.Draft != nil && *request.Draft
	sourceID := request.DraftSourceID()
	if !requested && sourceID == "" {
		return nil
	}
	switch a.protocol {
	case dto.VideoUpstreamProtocolViduModelArkV3, dto.VideoUpstreamProtocolFunCloudModelArkV3,
		dto.VideoUpstreamProtocolMoxingModelArkV1, dto.VideoUpstreamProtocolModelArkV3Volcengine,
		dto.VideoUpstreamProtocolModelArkV3BytePlus, dto.VideoUpstreamProtocolNexhinaModelArkV3,
		dto.VideoUpstreamProtocolArkMediaV1:
	default:
		return service.TaskErrorWrapperLocal(errors.New("draft operation is not supported by this video protocol"), "unsupported_parameter", http.StatusBadRequest)
	}
	plugin := pinnedSeedanceExtension(c)
	if plugin == nil || !plugin.Engine.HasExport("modelArkDraftV3") {
		return service.TaskErrorWrapperLocal(errors.New("draft operation is unavailable in the active video adapter"), "unsupported_parameter", http.StatusBadRequest)
	}
	if sourceID != "" {
		tasks, _ := common.GetContextKeyType[[]*model.Task](c, constant.ContextKeyOriginTasks)
		if len(tasks) != 1 || tasks[0] == nil || len(info.OriginTasks) != 1 || tasks[0].TaskID != sourceID ||
			info.OriginTasks[0].TaskID != sourceID || info.OriginTasks[0].UpstreamTaskID != tasks[0].PrivateData.UpstreamTaskID ||
			tasks[0].ChannelId != info.ChannelId || tasks[0].Properties.UpstreamModelName != providerModelFromRelayInfo(info, request.Model) ||
			tasks[0].PrivateData.VideoUpstreamProtocol != a.protocol ||
			strings.TrimRight(tasks[0].PrivateData.VideoUpstreamQueryBaseURL, "/") != strings.TrimRight(a.baseURL, "/") ||
			tasks[0].PrivateData.VideoUpstreamProxy != info.ChannelSetting.Proxy ||
			tasks[0].PrivateData.Key == "" || tasks[0].PrivateData.Key != a.apiKey {
			return service.TaskErrorWrapperLocal(errors.New("the requested draft task cannot be used"), "invalid_draft_task", http.StatusBadRequest)
		}
		info.Action = info.OriginTasks[0].Action
	}
	if requested {
		if request.Resolution == nil {
			request.Resolution = common.GetPointer("480p")
		} else if strings.TrimSpace(*request.Resolution) == "" {
			return service.TaskErrorWrapperLocal(errors.New("draft resolution must not be empty"), "invalid_video_parameter", http.StatusBadRequest)
		}
	}
	info.VideoDraftRequested = requested
	info.VideoDraftSourceID = sourceID
	return nil
}

// Final pricing inherits input facts, but never the source price, token
// budget, or measured token charge. The target resolution belongs to this job.
func draftFinalProbe(c *gin.Context, info *relaycommon.RelayInfo, resolution string, provider dto.VideoUpstreamProtocol) (map[string]any, error) {
	tasks, _ := common.GetContextKeyType[[]*model.Task](c, constant.ContextKeyOriginTasks)
	if len(tasks) != 1 || tasks[0].TaskID != info.VideoDraftSourceID ||
		tasks[0].PrivateData.AsyncBilling == nil || tasks[0].PrivateData.AsyncBilling.TieredSnapshot == nil {
		return nil, fmt.Errorf("the draft task has no frozen pricing facts")
	}
	facts := tasks[0].PrivateData.AsyncBilling.TieredSnapshot.UsageFacts
	probe := map[string]any{"resolution": resolution}
	for _, name := range []string{"has_video_input", "generate_audio"} {
		value, ok := facts[name].(bool)
		if !ok {
			return nil, fmt.Errorf("the draft task has incomplete pricing facts")
		}
		probe[name] = value
	}
	for _, name := range []string{"input_mode", "control_mode"} {
		value, ok := facts[name].(string)
		if !ok || value == "" {
			return nil, fmt.Errorf("the draft task has incomplete pricing facts")
		}
		probe[name] = value
	}
	seconds, ok := facts["duration_seconds"].(float64)
	if !ok || seconds <= 0 || seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("the draft task has no valid frozen duration")
	}
	probe["duration_seconds"] = seconds
	if provider == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
		probe["billing_mode"] = "per-token"
	}
	return probe, nil
}

// Verify the full final body against the trusted source reference. A plugin
// may neither replace that reference nor add inherited input fields.
func (a *TaskAdaptor) finishDraftFinalConversion(c *gin.Context, info *relaycommon.RelayInfo, plugin *jsplugin.LoadedPlugin, result any, request map[string]any, upstreamModel string) (*seedancePluginCreateConversion, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid draft video conversion")
	}
	body, ok := object["body"].(map[string]any)
	probe, probeOK := object["probe"].(map[string]any)
	if !ok || !probeOK || len(probe) != 0 {
		return nil, fmt.Errorf("invalid draft video body")
	}
	expected := map[string]any{
		"resolution": request["resolution"],
		"content": []taskdto.ModelArkVideoContent{{
			Type: "draft_task", DraftTask: &taskdto.ModelArkDraftTask{ID: info.OriginTasks[0].UpstreamTaskID},
		}},
	}
	if a.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
		expected["real_person_mode"] = true
	}
	if err := validateOfficialPluginBillingRequest(body, expected, upstreamModel); err != nil {
		return nil, err
	}
	path, _ := object["createPath"].(string)
	query, _ := object["queryPath"].(string)
	if err := dto.ValidateVideoUpstreamURL(a.baseURL, path, query); err != nil {
		return nil, err
	}
	tasks, _ := common.GetContextKeyType[[]*model.Task](c, constant.ContextKeyOriginTasks)
	if len(tasks) != 1 || tasks[0] == nil || !draftTaskPathsMatch(tasks[0], path, query) {
		return nil, relaycommon.NewVideoContractError("invalid_draft_task", "the requested draft task cannot be used")
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	a.createPath = path
	info.ChannelOtherSettings.VideoUpstreamCreatePath = path
	info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate = query
	a.pluginCreate = &seedancePluginCreateConversion{plugin: plugin, body: data, probe: probe}
	return a.pluginCreate, nil
}

func draftTaskPathsMatch(source *model.Task, createPath, queryPath string) bool {
	frozen := source.PrivateData
	if frozen.VideoUpstreamQueryPathTemplate == "" || frozen.VideoUpstreamQueryPathTemplate != queryPath {
		return false
	}
	if frozen.VideoDraftCreatePath != "" {
		return frozen.VideoDraftCreatePath == createPath
	}
	// Initial drafts froze only the query path. Reuse them with the matching
	// registered endpoint pair, never with a newly supplied custom create path.
	baselineCreate, baselineQuery := frozen.VideoUpstreamProtocol.TransportPaths(source.Properties.UpstreamModelName)
	if baselineCreate == "" && seedanceOfficialUsageProtocol(frozen.VideoUpstreamProtocol) {
		baselineCreate = officialVideoCreatePath
		baselineQuery = baselineCreate + "/{task_id}"
	}
	return baselineQuery == queryPath && baselineCreate == createPath
}
