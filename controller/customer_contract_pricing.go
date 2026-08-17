package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func respondCustomerContractPricing(c *gin.Context) bool {
	if !common.GetContextKeyBool(c, constant.ContextKeyContractMode) {
		return false
	}
	version, ok := common.GetContextKeyType[int64](c, constant.ContextKeyContractVersion)
	if !ok {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	snapshot, err := service.LoadCustomerContractSnapshot(c.GetInt("id"), version)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	pricing, err := service.BuildCustomerContractPricing(snapshot, userGroup)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
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
