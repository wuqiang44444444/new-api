package controller

import (
	"net/http"
	"slices"
	"sort"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func customerContractModelProjection(c *gin.Context) ([]string, []string, bool, error) {
	snapshot, err := tokenContractSnapshotForRequest(c)
	if err != nil || snapshot == nil {
		return nil, nil, false, err
	}
	rules, err := customerContractProjectionRules(c, snapshot)
	if err != nil {
		return nil, nil, true, err
	}
	models, groups := []string{}, []string{}
	ids := make([]int, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ChannelId)
	}
	batchIDs, err := model.CustomerContractBatchSourceIDs(ids)
	if err != nil {
		return nil, nil, true, err
	}
	for _, rule := range rules {
		if batchIDs[rule.ChannelId] || !service.ContractTokenModelAllowed(c, rule.PublicModel) {
			continue
		}
		if !slices.Contains(models, rule.PublicModel) {
			models = append(models, rule.PublicModel)
		}
		if !slices.Contains(groups, rule.RouteGroup) {
			groups = append(groups, rule.RouteGroup)
		}
	}
	sort.Strings(models)
	sort.Strings(groups)
	return models, groups, true, nil
}

func customerContractModelMetadata(c *gin.Context, models, groups []string) (map[string]*dto.PublicModelAPI, map[string]dto.OpenAIModels, error) {
	snapshot, err := tokenContractSnapshotForRequest(c)
	if err != nil {
		return nil, nil, err
	}
	if snapshot == nil {
		apis, err := model.GetPublicMediaModelAPIs(models, groups)
		return apis, nil, err
	}
	rules, err := customerContractProjectionRules(c, snapshot)
	if err != nil {
		return nil, nil, err
	}
	metadata, err := model.GetContractModelMetadata(rules)
	if err != nil {
		return nil, nil, err
	}
	apis := make(map[string]*dto.PublicModelAPI, len(metadata))
	for name, item := range metadata {
		apis[name] = item.API
	}
	return apis, metadata, nil
}

func customerContractSeedanceCatalog(c *gin.Context, catalog []model.SeedancePublicModel) ([]model.SeedancePublicModel, error) {
	snapshot, err := tokenContractSnapshotForRequest(c)
	if err != nil || snapshot == nil {
		return catalog, err
	}
	rules, err := customerContractProjectionRules(c, snapshot)
	if err != nil {
		return nil, err
	}
	channels, err := model.GetContractSourceChannels(rules)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool)
	for _, rule := range rules {
		channel := channels[rule.ChannelId]
		// The shared standard entry spans the typed video channels, so the
		// contract projection keeps rules from both.
		isTypedVideo := channel.Type == constant.ChannelTypeSeedanceLink || channel.Type == constant.ChannelTypeMiniMaxLink
		if isTypedVideo && service.ContractTokenModelAllowed(c, rule.PublicModel) {
			allowed[rule.PublicModel] = true
		}
	}
	filtered := make([]model.SeedancePublicModel, 0, len(catalog))
	for _, item := range catalog {
		if allowed[item.ModelName] {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func applyCustomerContractSeedanceModels(c *gin.Context, models []dto.OpenAIModels) ([]dto.OpenAIModels, error) {
	catalog, err := standardVideoPublicCatalog()
	if err != nil {
		return nil, err
	}
	catalog, err = customerContractSeedanceCatalog(c, catalog)
	if err != nil {
		return nil, err
	}
	visible := make(map[string]bool, len(models))
	for _, item := range models {
		visible[item.Id] = true
	}
	filtered := catalog[:0]
	for _, item := range catalog {
		if visible[item.ModelName] {
			filtered = append(filtered, item)
		}
	}
	return applyConfiguredSeedanceModels(models, filtered), nil
}

func customerContractModelVisible(c *gin.Context, publicModel string) bool {
	models, _, active, err := customerContractModelProjection(c)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return false
	}
	if active && (!slices.Contains(models, publicModel) || !retrieveModelHasBilling(c, publicModel)) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "invalid_request_error", "code": "model_not_found", "message": "合同范围内无可用模型 / Model is unavailable in this contract"}})
		return false
	}
	return true
}

// Only discovery projections reuse current eligibility within one response.
// Runtime retries continue to recheck live channel eligibility independently.
func customerContractProjectionRules(c *gin.Context, snapshot *model.ContractEntitySnapshot) ([]model.ContractEntityRule, error) {
	const key = "customer_contract_projection_rules"
	if value, exists := c.Get(key); exists {
		return value.([]model.ContractEntityRule), nil
	}
	rules, err := service.EffectiveContractRules(snapshot)
	if err != nil {
		return nil, err
	}
	c.Set(key, rules)
	return rules, nil
}
