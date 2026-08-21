package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
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
	models := channel.GetModels()
	if len(models) != 1 {
		return fmt.Errorf("CMCC Seedance channels require exactly one customer model")
	}
	customerModel := strings.TrimSpace(models[0])
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping); err != nil {
		return fmt.Errorf("CMCC Seedance requires one exact model_mapping entry")
	}
	if len(mapping) != 1 || strings.TrimSpace(mapping[customerModel]) != CMCCSeedance20ProviderModel {
		return fmt.Errorf("CMCC Seedance model_mapping must map customer model %q to %q", customerModel, CMCCSeedance20ProviderModel)
	}
	settings.AssetProviderProject = ""
	settings.AssetRegion = ""
	return nil
}
