package controller

import (
	"net/http"
	"time"

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

func ListSeedanceModels(c *gin.Context) {
	if listCustomerContractModelsFiltered(c, constant.ChannelTypeOpenAI, true) {
		return
	}
	groups, err := getModelListGroups(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "get user group failed"})
		return
	}

	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if !acceptUnsetRatioModel && c.GetInt("id") > 0 {
		userSettings, _ := model.GetUserSetting(c.GetInt("id"), false)
		acceptUnsetRatioModel = userSettings.AcceptUnsetRatioModel
	}
	modelLimitEnabled := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	modelLimit := map[string]bool{}
	if value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit); ok {
		modelLimit, _ = value.(map[string]bool)
	}

	catalog, err := model.GetConfiguredSeedancePublicModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "get Seedance model catalog failed"})
		return
	}
	catalogNames := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		catalogNames[item.ModelName] = struct{}{}
	}
	availableModels := make([]dto.OpenAIModels, 0, len(catalog))
	for _, modelName := range service.GetGroupsEnabledModels(groups.ownerGroups) {
		if _, exists := catalogNames[modelName]; !exists {
			continue
		}
		if modelLimitEnabled {
			matchingName := ratio_setting.FormatMatchingModelName(modelName)
			if !modelLimit[modelName] && !modelLimit[matchingName] {
				continue
			}
		}
		if !acceptUnsetRatioModel && !helper.HasModelBillingConfig(modelName) {
			continue
		}
		availableModels = append(availableModels, buildOpenAIModel(modelName, nil))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"object":  "list",
		"data":    applyConfiguredSeedanceModels(availableModels, catalog),
	})
}

func appendConfiguredSeedanceModels(models []dto.OpenAIModels) []dto.OpenAIModels {
	catalog, err := model.GetConfiguredSeedancePublicModels()
	if err != nil {
		common.SysLog("GetConfiguredSeedancePublicModels error: " + err.Error())
		return models
	}
	return applyConfiguredSeedanceModels(models, catalog)
}

func applyConfiguredSeedanceModels(models []dto.OpenAIModels, catalog []model.SeedancePublicModel) []dto.OpenAIModels {
	indexByName := make(map[string]int, len(models)+len(catalog))
	for i := range models {
		indexByName[models[i].Id] = i
	}
	for i := range catalog {
		index, available := indexByName[catalog[i].ModelName]
		if !available {
			models = append(models, dto.OpenAIModels{
				Id:      catalog[i].ModelName,
				Object:  "model",
				Created: 1626777600,
			})
			index = len(models) - 1
			indexByName[catalog[i].ModelName] = index
		}
		applySeedanceModelCatalog(&models[index], &catalog[i], available)
	}
	return models
}

func configuredSeedanceModel(c *gin.Context, modelName string) (dto.OpenAIModels, bool) {
	catalog, err := model.GetConfiguredSeedancePublicModels()
	if err != nil {
		common.SysLog("GetConfiguredSeedancePublicModels error: " + err.Error())
		return dto.OpenAIModels{}, false
	}
	for i := range catalog {
		if catalog[i].ModelName != modelName {
			continue
		}
		result := dto.OpenAIModels{Id: modelName, Object: "model", Created: 1626777600}
		applySeedanceModelCatalog(&result, &catalog[i], seedanceModelAvailableToCaller(c, modelName))
		return result, true
	}
	return dto.OpenAIModels{}, false
}

func respondConfiguredSeedanceModel(c *gin.Context, modelType int, modelName string) bool {
	seedanceModel, ok := configuredSeedanceModel(c, modelName)
	if !ok {
		return false
	}
	if modelType == constant.ChannelTypeAnthropic {
		c.JSON(http.StatusOK, dto.AnthropicModel{
			ID: seedanceModel.Id, CreatedAt: time.Unix(int64(seedanceModel.Created), 0).UTC().Format(time.RFC3339),
			DisplayName: seedanceModel.Id, Type: "model", Available: seedanceModel.Available,
			Availability: seedanceModel.Availability, API: seedanceModel.API,
		})
		return true
	}
	c.JSON(http.StatusOK, seedanceModel)
	return true
}

func seedanceModelAvailableToCaller(c *gin.Context, modelName string) bool {
	groups, err := getModelListGroups(c)
	if err != nil {
		return false
	}
	available := false
	for _, enabledModel := range service.GetGroupsEnabledModels(groups.ownerGroups) {
		if enabledModel == modelName {
			available = true
			break
		}
	}
	if !available {
		return false
	}

	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		modelLimit, valid := value.(map[string]bool)
		matchingName := ratio_setting.FormatMatchingModelName(modelName)
		if !ok || !valid || (!modelLimit[modelName] && !modelLimit[matchingName]) {
			return false
		}
	}
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

func applySeedanceModelCatalog(target *dto.OpenAIModels, catalog *model.SeedancePublicModel, available bool) {
	target.OwnedBy = "new-api"
	target.SupportedEndpointTypes = []constant.EndpointType{constant.EndpointTypeModelArkVideo}
	target.Available = common.GetPointer(available)
	target.Availability = "available"
	if !available {
		target.Availability = "restricted"
	}
	if !catalog.Enabled {
		target.Availability = "disabled"
	}
	api := catalog.API
	target.API = &api
}
