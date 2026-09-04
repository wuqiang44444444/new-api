package seedance

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/funcloud"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 官方协议内置路径（方案 §3.1），不参与渠道路径配置。
const officialVideoCreatePath = "/api/v3/contents/generations/tasks"

// videoCreatePath 返回创建请求路径：official 用内置路径，第三方协议用渠道配置的创建后缀。
func videoCreatePath(profile dto.VideoUpstreamProfile, configuredCreatePath string) (string, error) {
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial:
		return officialVideoCreatePath, nil
	case dto.VideoUpstreamProfileThirdPartyRelay,
		dto.VideoUpstreamProfileThirdPartyMoxingModelArk,
		dto.VideoUpstreamProfileThirdPartyReverseProxy,
		dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
		dto.VideoUpstreamProfileThirdPartyFunCloudSeedance:
		if strings.TrimSpace(configuredCreatePath) == "" {
			return "", fmt.Errorf("video_upstream_create_path is required for third-party profile")
		}
		return configuredCreatePath, nil
	default:
		return "", dto.ValidateVideoUpstreamProfile(profile)
	}
}

// videoTaskPath 返回查询请求路径：official 用内置路径，第三方协议用配置模板替换 {task_id}（path escape）。
func videoTaskPath(profile dto.VideoUpstreamProfile, configuredQueryTemplate, taskID string) (string, error) {
	escapedTaskID := url.PathEscape(taskID)
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial:
		return officialVideoCreatePath + "/" + escapedTaskID, nil
	case dto.VideoUpstreamProfileThirdPartyRelay,
		dto.VideoUpstreamProfileThirdPartyMoxingModelArk,
		dto.VideoUpstreamProfileThirdPartyReverseProxy,
		dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
		dto.VideoUpstreamProfileThirdPartyFunCloudSeedance:
		if strings.TrimSpace(configuredQueryTemplate) == "" {
			return "", fmt.Errorf("video_upstream_query_path_template is required for third-party profile")
		}
		return strings.Replace(configuredQueryTemplate, "{task_id}", escapedTaskID, 1), nil
	default:
		return "", dto.ValidateVideoUpstreamProfile(profile)
	}
}

// joinVideoUpstreamURL 拼接根地址与路径，移除根地址末尾多余 / 避免重复斜杠（方案 §5.2-5）。
func joinVideoUpstreamURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + path
}

// convertVideoCreateRequest 按协议转换创建请求体：第三方中转协议转换为统一媒体任务结构，其余透传。
func convertVideoCreateRequest(profile dto.VideoUpstreamProfile, body []byte) ([]byte, error) {
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial, dto.VideoUpstreamProfileThirdPartyReverseProxy,
		dto.VideoUpstreamProfileThirdPartyMoxingModelArk:
		return body, nil
	case dto.VideoUpstreamProfileThirdPartyRelay:
		return thirdparty.RelayCreateRequest(body)
	case dto.VideoUpstreamProfileThirdPartyFeicaiVideos:
		return nil, fmt.Errorf("the selected video adapter requires the typed capability path")
	case dto.VideoUpstreamProfileThirdPartyFunCloudSeedance:
		return nil, fmt.Errorf("the selected video adapter requires the typed capability path")
	default:
		return nil, dto.ValidateVideoUpstreamProfile(profile)
	}
}

// normalizeVideoCreateResponse 按协议归一化创建响应到内部 {"id": ...} 合同。
func normalizeVideoCreateResponse(profile dto.VideoUpstreamProfile, body []byte) ([]byte, error) {
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial:
		return body, nil
	case dto.VideoUpstreamProfileThirdPartyReverseProxy:
		return thirdparty.ReverseProxyCreateResponse(body)
	case dto.VideoUpstreamProfileThirdPartyRelay:
		return thirdparty.RelayCreateResponse(body)
	case dto.VideoUpstreamProfileThirdPartyMoxingModelArk:
		return thirdparty.RelayCreateResponse(body)
	case dto.VideoUpstreamProfileThirdPartyFeicaiVideos:
		return feicai.CreateResponse(body)
	case dto.VideoUpstreamProfileThirdPartyFunCloudSeedance:
		return funcloud.CreateResponse(body)
	default:
		return nil, dto.ValidateVideoUpstreamProfile(profile)
	}
}

// normalizeVideoTaskResponse 按协议归一化查询响应到现有 DoubaoVideo 轮询合同。
func normalizeVideoTaskResponse(
	profile dto.VideoUpstreamProfile,
	adapterVersion relaycommon.VideoSouthboundAdapterVersion,
	body []byte,
	expectedTaskID string,
	baseURL string,
	billingContext *relaycommon.VideoTaskBillingContext,
) ([]byte, error) {
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial:
		return body, nil
	case dto.VideoUpstreamProfileThirdPartyReverseProxy:
		return thirdparty.ReverseProxyTaskResponse(body)
	case dto.VideoUpstreamProfileThirdPartyRelay:
		if !adapterVersion.IsThirdPartyRelayV2() {
			return thirdparty.RelayTaskResponseV1(body)
		}
		return thirdparty.RelayTaskResponse(body, expectedTaskID)
	case dto.VideoUpstreamProfileThirdPartyMoxingModelArk:
		if !adapterVersion.IsMoxingModelArkV1() {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported video adapter revision"}
		}
		return thirdparty.RelayTaskResponse(body, expectedTaskID)
	case dto.VideoUpstreamProfileThirdPartyFeicaiVideos:
		if !adapterVersion.IsFeicaiVideos() {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported video adapter revision"}
		}
		return feicai.TaskResponse(body, expectedTaskID, feicai.TaskResponseContext{BaseURL: baseURL})
	case dto.VideoUpstreamProfileThirdPartyFunCloudSeedance:
		if !adapterVersion.IsFunCloudSeedanceV3() {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported video adapter revision"}
		}
		responseContext, err := funCloudTaskResponseContext(billingContext)
		if err != nil {
			return nil, err
		}
		return funcloud.TaskResponse(body, expectedTaskID, responseContext)
	default:
		return nil, dto.ValidateVideoUpstreamProfile(profile)
	}
}

// videoAdapterVersionFromTask 用渠道类型与创建时冻结的 adapter 版本快照解析南向协议版本。
func videoAdapterVersionFromTask(
	task *model.Task,
	channelType int,
	profile dto.VideoUpstreamProfile,
) (relaycommon.VideoSouthboundAdapterVersion, error) {
	version, err := relaycommon.ResolveVideoSouthboundAdapterVersion(channelType, profile, task.PrivateData.SouthboundAdapterVersion)
	if err != nil {
		return relaycommon.VideoSouthboundAdapterVersion{}, &relaycommon.UpstreamContractViolation{
			Reason: "invalid video adapter version",
		}
	}
	return version, nil
}

// videoUpstreamProfileFromTask 返回创建时冻结的传输协议；无快照的历史任务视为 official。
func videoUpstreamProfileFromTask(task *model.Task) (dto.VideoUpstreamProfile, error) {
	profile := task.PrivateData.VideoUpstreamProfile
	if profile == "" {
		return dto.VideoUpstreamProfileOfficial, nil
	}
	if err := dto.ValidateVideoUpstreamProfile(profile); err != nil {
		return "", err
	}
	return profile, nil
}

// frozenVideoBillingContext 只读创建时冻结的计费事实（Provider 模型、计费探针 Body 与估算 token 上限）。
// Provider adapter 在归一化终态响应时可能使用它们；轮询不得从当前渠道或价格配置重建这些事实。
func frozenVideoBillingContext(task *model.Task) *relaycommon.VideoTaskBillingContext {
	if task == nil {
		return &relaycommon.VideoTaskBillingContext{}
	}
	context := &relaycommon.VideoTaskBillingContext{ProviderModel: task.Properties.UpstreamModelName}
	if task.PrivateData.AsyncBilling == nil {
		return context
	}
	context.EstimatedTokens = task.PrivateData.AsyncBilling.EstimatedTokens
	if task.PrivateData.AsyncBilling.BillingProbe == nil {
		return context
	}
	context.BillingProbeBody = append([]byte(nil), task.PrivateData.AsyncBilling.BillingProbe.Body...)
	return context
}
