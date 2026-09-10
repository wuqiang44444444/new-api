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
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// tokenContractSnapshotForRequest resolves the API key's bound contract for
// dashboard read endpoints. A nil snapshot with a nil error means the key has
// no active contract binding and native behavior applies; a non-nil error is a
// load failure the caller must fail closed on.
func tokenContractSnapshotForRequest(c *gin.Context) (*model.ContractEntitySnapshot, error) {
	contractId, _ := common.GetContextKeyType[int](c, constant.ContextKeyTokenContractId)
	if contractId <= 0 {
		return nil, nil
	}
	authVersion, _ := common.GetContextKeyType[int64](c, constant.ContextKeyAuthVersion)
	snapshot, err := service.LoadContractEntityForRequest(c.GetInt("id"), authVersion, contractId)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || !snapshot.Enabled {
		return nil, nil
	}
	if err := model.RefreshContractEntityAvailability(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func listCustomerContractModels(c *gin.Context, modelType int) bool {
	return listCustomerContractModelsFiltered(c, modelType, false)
}

func listCustomerContractModelsFiltered(c *gin.Context, modelType int, seedanceOnly bool) bool {
	snapshot, err := tokenContractSnapshotForRequest(c)
	if err != nil {
		respondCustomerContractModelLoadError(c)
		return true
	}
	if snapshot == nil {
		return false
	}

	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if !acceptUnsetRatioModel {
		userSettings, _ := model.GetUserSetting(c.GetInt("id"), false)
		acceptUnsetRatioModel = userSettings.AcceptUnsetRatioModel
	}
	modelLimitEnabled := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	modelLimit := map[string]bool{}
	if value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit); exists {
		modelLimit, _ = value.(map[string]bool)
	}

	seedanceCatalog, _ := model.GetConfiguredSeedancePublicModels()
	seedanceByModel := make(map[string]model.SeedancePublicModel, len(seedanceCatalog))
	for _, item := range seedanceCatalog {
		seedanceByModel[item.ModelName] = item
	}
	channelIDs := make([]int, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		channelIDs = append(channelIDs, rule.ChannelId)
	}
	batchChannels, err := model.BatchContractChannelIDs(channelIDs)
	if err != nil {
		respondCustomerContractModelLoadError(c)
		return true
	}
	models := make([]dto.OpenAIModels, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		if !rule.Available {
			continue
		}
		if modelLimitEnabled {
			matchingName := ratio_setting.FormatMatchingModelName(rule.PublicModel)
			if !modelLimit[rule.PublicModel] && !modelLimit[matchingName] {
				continue
			}
		}
		var hasPrice bool
		if batchChannels[rule.ChannelId] {
			_, hasPrice = billing_setting.GetBatchBillingExpr(rule.PublicModel)
		} else {
			hasPrice = helper.HasModelBillingConfig(rule.PublicModel)
		}
		if !acceptUnsetRatioModel && !hasPrice {
			continue
		}
		_, isSeedance := seedanceByModel[rule.PublicModel]
		if seedanceOnly && !isSeedance {
			continue
		}
		item := dto.OpenAIModels{
			Id: rule.PublicModel, Object: "model", Created: 1626777600, OwnedBy: "new-api",
			SupportedEndpointTypes: model.GetModelSupportEndpointTypes(rule.PublicModel),
		}
		if catalog, exists := seedanceByModel[rule.PublicModel]; exists {
			applySeedanceModelCatalog(&item, &catalog, true)
		}
		models = append(models, item)
	}
	respondCustomerContractModels(c, modelType, models)
	return true
}

func retrieveCustomerContractModel(c *gin.Context, modelType int, modelName string) bool {
	snapshot, err := tokenContractSnapshotForRequest(c)
	if err != nil {
		respondCustomerContractModelLoadError(c)
		return true
	}
	if snapshot == nil {
		return false
	}
	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		limits, valid := value.(map[string]bool)
		matchingName := ratio_setting.FormatMatchingModelName(modelName)
		if !exists || !valid || (!limits[modelName] && !limits[matchingName]) {
			respondCustomerContractModelNotFound(c)
			return true
		}
	}
	for _, rule := range snapshot.Rules {
		if rule.PublicModel != modelName || !rule.Available {
			continue
		}
		models := []dto.OpenAIModels{{
			Id: modelName, Object: "model", Created: 1626777600, OwnedBy: "new-api",
			SupportedEndpointTypes: model.GetModelSupportEndpointTypes(modelName),
		}}
		catalog, _ := model.GetConfiguredSeedancePublicModels()
		for i := range catalog {
			if catalog[i].ModelName == modelName {
				applySeedanceModelCatalog(&models[0], &catalog[i], true)
				break
			}
		}
		respondCustomerContractRetrievedModel(c, modelType, models[0])
		return true
	}
	respondCustomerContractModelNotFound(c)
	return true
}

func respondCustomerContractRetrievedModel(c *gin.Context, modelType int, item dto.OpenAIModels) {
	if modelType == constant.ChannelTypeAnthropic {
		c.JSON(http.StatusOK, dto.AnthropicModel{
			ID: item.Id, CreatedAt: time.Unix(int64(item.Created), 0).UTC().Format(time.RFC3339),
			DisplayName: item.Id, Type: "model", Available: item.Available,
			Availability: item.Availability, API: item.API,
		})
		return
	}
	c.JSON(http.StatusOK, item)
}

func respondCustomerContractModelNotFound(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"error": gin.H{
		"message": "The requested model does not exist", "type": "invalid_request_error",
		"param": "model", "code": "model_not_found",
	}})
}

func respondCustomerContractModels(c *gin.Context, modelType int, models []dto.OpenAIModels) {
	switch modelType {
	case constant.ChannelTypeAnthropic:
		items := make([]dto.AnthropicModel, len(models))
		for i, item := range models {
			items[i] = dto.AnthropicModel{
				ID: item.Id, CreatedAt: time.Unix(int64(item.Created), 0).UTC().Format(time.RFC3339),
				DisplayName: item.Id, Type: "model", Available: item.Available,
				Availability: item.Availability, API: item.API,
			}
		}
		firstID, lastID := "", ""
		if len(items) > 0 {
			firstID, lastID = items[0].ID, items[len(items)-1].ID
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "first_id": firstID, "has_more": false, "last_id": lastID})
	case constant.ChannelTypeGemini:
		items := make([]dto.GeminiModel, len(models))
		for i, item := range models {
			items[i] = dto.GeminiModel{
				Name: item.Id, DisplayName: item.Id, Available: item.Available,
				Availability: item.Availability, API: item.API,
			}
		}
		c.JSON(http.StatusOK, gin.H{"models": items, "nextPageToken": nil})
	default:
		c.JSON(http.StatusOK, gin.H{"success": true, "data": models, "object": "list"})
	}
}

func respondCustomerContractModelLoadError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false, "message": "customer contract is temporarily unavailable",
	})
}

func listDashboardContractModels(c *gin.Context) bool {
	contracts, err := model.ListContractEntitiesForUser(c.GetInt("id"), false)
	if err != nil {
		respondCustomerContractModelLoadError(c)
		return true
	}
	if len(contracts) > 0 {
		models := make([]string, 0)
		seen := make(map[string]struct{})
		for i := range contracts {
			if !contracts[i].Enabled {
				continue
			}
			if err := model.RefreshContractEntityAvailability(&contracts[i]); err != nil {
				respondCustomerContractModelLoadError(c)
				return true
			}
			for _, rule := range contracts[i].Rules {
				if !rule.Available {
					continue
				}
				if _, exists := seen[rule.PublicModel]; exists {
					continue
				}
				seen[rule.PublicModel] = struct{}{}
				models = append(models, rule.PublicModel)
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": map[int][]string{0: models}})
		return true
	}
	return false
}
