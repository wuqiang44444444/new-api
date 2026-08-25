package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/dto"
)

const CMCCAICCAssetBaseURL = "https://ecloud.10086.cn/api/openapi-maas/exp/aicc/v2"
const CMCCSeedance20ProviderModel = "doubao-seedance-2.0"

func validateCMCCSeedanceChannel(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3CMCC &&
		settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolCMCCAICCV2 {
		return nil
	}
	if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3CMCC {
		return fmt.Errorf("CMCC AICC assets require the CMCC ModelArk V3 video protocol")
	}
	if err := dto.ValidateVideoUpstreamURL(
		channel.GetBaseURL(),
		"/api/v3/contents/generations/tasks",
		"/api/v3/contents/generations/tasks/{task_id}",
	); err != nil {
		return err
	}
	if _, err := resolveSeedanceChannelProviderModels(
		channel,
		settings.VideoUpstreamProtocol,
		map[string]struct{}{CMCCSeedance20ProviderModel: {}},
	); err != nil {
		return err
	}
	settings.AssetProviderProject = ""
	settings.AssetRegion = ""
	return nil
}
