package doubao

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao/thirdparty"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao/thirdparty/funcloud"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao/thirdparty/mediaarrays"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const (
	videoUpstreamProfileBodyKey       = "video_upstream_profile"
	videoUpstreamQueryTemplateBodyKey = "video_upstream_query_path_template"
	videoUpstreamAdapterVersionKey    = "video_upstream_adapter_version"
	videoUpstreamImplementationIDKey  = "video_upstream_implementation_id"
)

// 官方协议内置路径（方案 §3.1），不参与渠道路径配置。
const officialVideoCreatePath = "/api/v3/contents/generations/tasks"

// videoCreatePath 返回创建请求路径：official 用内置路径，第三方协议用渠道配置的创建后缀。
func videoCreatePath(profile dto.VideoUpstreamProfile, configuredCreatePath string) (string, error) {
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial:
		return officialVideoCreatePath, nil
	case dto.VideoUpstreamProfileThirdPartyRelay,
		dto.VideoUpstreamProfileThirdPartyReverseProxy,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2:
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
		dto.VideoUpstreamProfileThirdPartyReverseProxy,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2:
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
	case "", dto.VideoUpstreamProfileOfficial, dto.VideoUpstreamProfileThirdPartyReverseProxy:
		return body, nil
	case dto.VideoUpstreamProfileThirdPartyRelay:
		return thirdparty.RelayCreateRequest(body)
	case dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays:
		return nil, fmt.Errorf("JSON video media-arrays request must use the typed capability path")
	case dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2:
		return nil, fmt.Errorf("FunCloud video request must use the typed capability path")
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
	case dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays:
		return mediaarrays.CreateResponse(body)
	case dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2:
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
	implementationID string,
	responseContext mediaarrays.TaskResponseContext,
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
		return thirdparty.RelayTaskResponse(body, expectedTaskID, thirdparty.RelayTaskResponseContext{
			IncludeVerifiedUsage: false,
		})
	case dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays:
		if !adapterVersion.IsJSONVideoMediaArraysV2() {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported video adapter revision"}
		}
		return mediaarrays.TaskResponse(body, expectedTaskID, responseContext)
	case dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2:
		if !adapterVersion.IsFunCloudSeedanceV2() {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported video adapter revision"}
		}
		return funcloud.TaskResponse(body, expectedTaskID)
	default:
		return nil, dto.ValidateVideoUpstreamProfile(profile)
	}
}

func videoImplementationIDFromFetchBody(body map[string]any) (string, error) {
	value, ok := body[videoUpstreamImplementationIDKey]
	if !ok || value == nil {
		return "", nil
	}
	implementationID, ok := value.(string)
	if !ok {
		return "", &relaycommon.UpstreamContractViolation{Reason: "invalid Link implementation ID type"}
	}
	return strings.TrimSpace(implementationID), nil
}

func videoAdapterVersionFromFetchBody(
	body map[string]any,
	channelType int,
	profile dto.VideoUpstreamProfile,
) (relaycommon.VideoSouthboundAdapterVersion, error) {
	value, ok := body[videoUpstreamAdapterVersionKey]
	frozen := ""
	if ok && value != nil {
		var valid bool
		frozen, valid = value.(string)
		if !valid {
			return relaycommon.VideoSouthboundAdapterVersion{}, &relaycommon.UpstreamContractViolation{
				Reason: "invalid video adapter version type",
			}
		}
	}
	version, err := relaycommon.ResolveVideoSouthboundAdapterVersion(channelType, profile, frozen)
	if err != nil {
		return relaycommon.VideoSouthboundAdapterVersion{}, &relaycommon.UpstreamContractViolation{
			Reason: "invalid video adapter version",
		}
	}
	return version, nil
}

// videoProfileFromFetchBody 从轮询请求体解析 profile，缺失时视为 official。
func videoProfileFromFetchBody(body map[string]any) (dto.VideoUpstreamProfile, error) {
	value, ok := body[videoUpstreamProfileBodyKey]
	if !ok || value == nil {
		return dto.VideoUpstreamProfileOfficial, nil
	}

	var profile dto.VideoUpstreamProfile
	switch typed := value.(type) {
	case dto.VideoUpstreamProfile:
		profile = typed
	case string:
		profile = dto.VideoUpstreamProfile(typed)
	default:
		return "", fmt.Errorf("invalid video upstream profile type %T", value)
	}
	if profile == "" {
		return dto.VideoUpstreamProfileOfficial, nil
	}
	if err := dto.ValidateVideoUpstreamProfile(profile); err != nil {
		return "", err
	}
	return profile, nil
}

// videoQueryTemplateFromFetchBody 读取轮询传入的查询路径模板快照，缺失时返回空（official 走内置路径）。
func videoQueryTemplateFromFetchBody(body map[string]any) string {
	value, ok := body[videoUpstreamQueryTemplateBodyKey]
	if !ok || value == nil {
		return ""
	}
	template, _ := value.(string)
	return template
}
