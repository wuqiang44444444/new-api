package controller

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type modelArkVideoCapabilityCatalogItem struct {
	ID                string                                  `json:"id"`
	Object            string                                  `json:"object"`
	Available         bool                                    `json:"available"`
	Published         bool                                    `json:"published"`
	VisibleInV1Models bool                                    `json:"visible_in_v1_models"`
	Capability        model.ModelArkVideoCapabilityProjection `json:"capability"`
}

type modelArkVideoCapabilityCatalog struct {
	Object string                               `json:"object"`
	Data   []modelArkVideoCapabilityCatalogItem `json:"data"`
}

// ModelArkVideoCapabilities exposes every registered ModelArk video candidate
// plus customer-model aliases published for the authenticated token. Alias
// projections keep the customer model identity and do not expose their Link SKU,
// Provider implementation, or connection details. Availability remains
// independent from code registration.
func ModelArkVideoCapabilities(c *gin.Context) {
	groups, err := getModelListGroups(c)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to resolve model access")
		return
	}
	visibleModels := modelArkVisibleModels(c, groups.ownerGroups)
	publications, err := model.ListLinkModelPublications(
		model.LinkContractNamespaceDefault,
		model.LinkRouteFamilyModelArkVideo,
		"",
	)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to resolve model publications")
		return
	}
	published := make(map[string]bool, len(publications))
	available := make(map[string]bool, len(publications))
	for _, publication := range publications {
		published[publication.CustomerModel] = true
	}
	for _, group := range groups.ownerGroups {
		availabilities, availabilityErr := model.GetLinkModelPublicationAvailabilities(publications, group)
		if availabilityErr != nil {
			modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to resolve model availability")
			return
		}
		for i, availability := range availabilities {
			if availability.CurrentlyFulfillable && !availability.RoutingConflict {
				available[publications[i].CustomerModel] = true
			}
		}
	}

	requestedModel := strings.TrimSpace(c.Query("model"))
	projections := model.RegisteredModelArkVideoCapabilityProjection()
	projectionBySKU := make(map[string]model.ModelArkVideoCapabilityProjection, len(projections))
	itemsByCustomerModel := make(map[string]modelArkVideoCapabilityCatalogItem, len(projections)+len(publications))
	for _, capability := range projections {
		projectionBySKU[capability.PublicModel] = capability
		visible := visibleModels[capability.PublicModel]
		itemsByCustomerModel[capability.PublicModel] = modelArkVideoCapabilityCatalogItem{
			ID:                capability.PublicModel,
			Object:            "model.capability",
			Available:         visible && available[capability.PublicModel],
			Published:         published[capability.PublicModel],
			VisibleInV1Models: visible,
			Capability:        capability,
		}
	}
	for _, publication := range publications {
		customerModel := strings.TrimSpace(publication.CustomerModel)
		capability, registered := projectionBySKU[strings.TrimSpace(publication.LinkSKU)]
		if customerModel == "" || !registered {
			continue
		}
		capability.PublicModel = customerModel
		visible := visibleModels[customerModel]
		itemsByCustomerModel[customerModel] = modelArkVideoCapabilityCatalogItem{
			ID:                customerModel,
			Object:            "model.capability",
			Available:         visible && available[customerModel],
			Published:         true,
			VisibleInV1Models: visible,
			Capability:        capability,
		}
	}
	response := modelArkVideoCapabilityCatalog{
		Object: "list",
		Data:   make([]modelArkVideoCapabilityCatalogItem, 0, len(itemsByCustomerModel)),
	}
	if requestedModel != "" {
		item, exists := itemsByCustomerModel[requestedModel]
		if !exists {
			modelArkVideoError(c, http.StatusNotFound, "model_not_found", "model capability not found")
			return
		}
		response.Data = append(response.Data, item)
	} else {
		for _, item := range itemsByCustomerModel {
			response.Data = append(response.Data, item)
		}
		slices.SortFunc(response.Data, func(left, right modelArkVideoCapabilityCatalogItem) int {
			return strings.Compare(left.ID, right.ID)
		})
	}
	c.Header("Cache-Control", "private, no-cache")
	c.JSON(http.StatusOK, response)
}

func modelArkVisibleModels(c *gin.Context, groups []string) map[string]bool {
	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if !acceptUnsetRatioModel && c.GetInt("id") > 0 {
		userSettings, _ := model.GetUserSetting(c.GetInt("id"), false)
		acceptUnsetRatioModel = userSettings.AcceptUnsetRatioModel
	}
	modelLimitEnabled := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	modelLimits := map[string]bool{}
	if modelLimitEnabled {
		if value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit); ok {
			modelLimits, _ = value.(map[string]bool)
		}
		if modelLimits == nil {
			modelLimits = map[string]bool{}
		}
	}
	visible := make(map[string]bool)
	for _, modelName := range service.GetGroupsEnabledModels(groups) {
		if modelLimitEnabled {
			matchingName := ratio_setting.FormatMatchingModelName(modelName)
			if !modelLimits[modelName] && !modelLimits[matchingName] {
				continue
			}
		}
		if !acceptUnsetRatioModel && !helper.HasModelBillingConfig(modelName) {
			continue
		}
		visible[modelName] = true
	}
	return visible
}
