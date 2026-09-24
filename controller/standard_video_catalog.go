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

// standardVideoPublicCatalog merges the typed video projections into one
// catalog with the same public contract. A broken configured declaration
// fails the catalog explicitly rather than silently removing customer models.
func standardVideoPublicCatalog() ([]model.SeedancePublicModel, error) {
	catalog, err := model.GetConfiguredSeedancePublicModels()
	if err != nil {
		return nil, err
	}
	minimaxModels, err := model.GetConfiguredMiniMaxPublicModels()
	if err != nil {
		return nil, err
	}
	merged := make([]model.SeedancePublicModel, 0, len(catalog)+len(minimaxModels))
	indexByName := make(map[string]int, len(catalog)+len(minimaxModels))
	for i := range catalog {
		indexByName[catalog[i].ModelName] = len(merged)
		merged = append(merged, catalog[i])
	}
	for i := range minimaxModels {
		if index, exists := indexByName[minimaxModels[i].ModelName]; exists {
			// Cross-type uniqueness is enforced at save/enable time; a
			// disabled duplicate keeps the enabled entry's projection.
			if minimaxModels[i].Enabled && !merged[index].Enabled {
				merged[index] = minimaxModels[i]
			}
			continue
		}
		indexByName[minimaxModels[i].ModelName] = len(merged)
		merged = append(merged, minimaxModels[i])
	}
	return merged, nil
}

// ListStandardVideoModels serves the ModelArk model directory across the
// typed video channels with the same filtering and public contract as the
// Seedance-only listing it extends.
func ListStandardVideoModels(c *gin.Context) {
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

	catalog, err := standardVideoPublicCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "get standard video model catalog failed"})
		return
	}
	catalog, err = customerContractSeedanceCatalog(c, catalog)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
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

// appendConfiguredStandardVideoModels feeds the generic model lists with the
// combined typed video projections.
func appendConfiguredStandardVideoModels(models []dto.OpenAIModels) []dto.OpenAIModels {
	catalog, err := standardVideoPublicCatalog()
	if err != nil {
		common.SysLog("standard video catalog error: " + err.Error())
		return models
	}
	return applyConfiguredSeedanceModels(models, catalog)
}

// configuredStandardVideoModel resolves one model's typed projection from
// the combined catalog.
func configuredStandardVideoModel(c *gin.Context, modelName string) (dto.OpenAIModels, bool) {
	catalog, err := standardVideoPublicCatalog()
	if err != nil {
		if service.ActiveCustomerContract(c) != nil {
			respondCustomerContractPricingLoadError(c)
		}
		common.SysLog("standard video catalog error: " + err.Error())
		return dto.OpenAIModels{}, false
	}
	catalog, err = customerContractSeedanceCatalog(c, catalog)
	if err != nil {
		if service.ActiveCustomerContract(c) != nil {
			respondCustomerContractPricingLoadError(c)
		}
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

// respondConfiguredStandardVideoModel answers single-model retrieval with
// the combined typed projection; it keeps the Seedance-first call order and
// adds the MiniMax subset.
func respondConfiguredStandardVideoModel(c *gin.Context, modelType int, modelName string) bool {
	seedanceModel, ok := configuredStandardVideoModel(c, modelName)
	if !ok {
		return c.IsAborted()
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
