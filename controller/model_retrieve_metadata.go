package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func modelMetadataForRetrieve(c *gin.Context, modelName string) (dto.OpenAIModels, bool) {
	_, static := openAIModelsMap[modelName]
	groups, err := getModelListGroups(c)
	if err != nil {
		if static {
			return buildOpenAIModel(modelName, nil), true
		}
		return dto.OpenAIModels{}, false
	}

	if !static {
		visible := false
		for _, candidate := range service.GetGroupsEnabledModels(groups.ownerGroups) {
			if candidate == modelName {
				visible = true
				break
			}
		}
		if !visible || !retrieveModelAllowedByToken(c, modelName) || !retrieveModelHasBilling(c, modelName) {
			return dto.OpenAIModels{}, false
		}
	}

	ownerByModel := getPreferredModelOwners([]string{modelName}, groups.ownerGroups)
	result := buildOpenAIModel(modelName, ownerByModel)
	if apiByModel, apiErr := model.GetPublicMediaModelAPIs([]string{modelName}, groups.ownerGroups); apiErr == nil {
		applyPublicMediaMetadata(&result, apiByModel[modelName])
	}
	return result, true
}

func applyPublicMediaMetadata(target *dto.OpenAIModels, api *dto.PublicModelAPI) {
	if target == nil || api == nil {
		return
	}
	target.API = api
	endpoint := constant.EndpointType("")
	if api.Image != nil {
		endpoint = constant.EndpointTypeImageGeneration
	} else if api.Video != nil && api.Video.Protocol == "openai_videos" {
		endpoint = constant.EndpointTypeOpenAIVideo
	}
	if endpoint == "" {
		return
	}
	for _, existing := range target.SupportedEndpointTypes {
		if existing == endpoint {
			return
		}
	}
	target.SupportedEndpointTypes = append([]constant.EndpointType{endpoint}, target.SupportedEndpointTypes...)
}

func retrieveModelAllowedByToken(c *gin.Context, modelName string) bool {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return true
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	modelLimit, valid := value.(map[string]bool)
	if !ok || !valid {
		return false
	}
	matchingName := ratio_setting.FormatMatchingModelName(modelName)
	return modelLimit[modelName] || modelLimit[matchingName]
}

func retrieveModelHasBilling(c *gin.Context, modelName string) bool {
	if operation_setting.SelfUseModeEnabled {
		return true
	}
	if c.GetInt("id") > 0 {
		userSettings, _ := model.GetUserSetting(c.GetInt("id"), false)
		if userSettings.AcceptUnsetRatioModel {
			return true
		}
	}
	return helper.HasModelBillingConfig(modelName)
}
