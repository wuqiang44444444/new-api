package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func respondCustomerContractPricing(c *gin.Context) bool {
	snapshot, err := tokenContractSnapshotForRequest(c)
	// Session pricing has no implicit default contract. An explicit selection is
	// owner-checked; API keys always keep their own binding.
	if c.GetInt("token_id") == 0 && c.Query("contract_id") != "" {
		id, parseErr := strconv.Atoi(c.Query("contract_id"))
		if parseErr != nil || id <= 0 {
			respondCustomerContractPricingLoadError(c)
			return true
		}
		if _, ownerErr := model.GetContractEntityOwnedByUser(id, c.GetInt("id")); ownerErr != nil {
			respondCustomerContractPricingLoadError(c)
			return true
		}
		snapshot, err = model.GetContractEntitySnapshot(id, true)
		if err == nil && !snapshot.Enabled {
			respondCustomerContractPricingLoadError(c)
			return true
		}
	}
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	if snapshot == nil {
		return false
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" {
		user, loadErr := model.GetUserCache(c.GetInt("id"))
		if loadErr != nil {
			respondCustomerContractPricingLoadError(c)
			return true
		}
		userGroup = user.Group
	}
	pricing, err := service.BuildContractEntityPricing(snapshot, userGroup)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	// 只读展示投影：合同倍率仍由展示层单独应用，不并入投影。
	for i := range pricing {
		service.AttachPricingBillingDisplayOne(&pricing[i].Pricing)
	}
	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            []model.PricingVendor{},
		"group_ratio":        gin.H{service.CustomerContractPublicPricingGroup: 1},
		"usable_group":       gin.H{service.CustomerContractPublicPricingGroup: "Contract"},
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        []string{},
		"pricing_version":    "customer-contract",
	})
	return true
}

func respondCustomerContractPricingLoadError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"message": "customer contract pricing is temporarily unavailable",
	})
}
