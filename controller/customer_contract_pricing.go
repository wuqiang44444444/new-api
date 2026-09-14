package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// tokenContractSnapshotForRequest resolves the API key's bound contract for
// dashboard read endpoints. A nil snapshot with a nil error means the key has
// no active contract binding and native behavior applies; a non-nil error is
// a load failure the caller must fail closed on.
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
	return snapshot, nil
}

func respondCustomerContractPricingLoadError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"message": "customer contract pricing is temporarily unavailable",
	})
}

func respondCustomerContractPricing(c *gin.Context) bool {
	snapshot, err := tokenContractSnapshotForRequest(c)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	// Session pricing has no implicit default contract. An explicit selection
	// is owner-checked; API keys always keep their own binding.
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
		snapshot, err = model.GetContractEntitySnapshot(id, false)
		if err != nil {
			respondCustomerContractPricingLoadError(c)
			return true
		}
		if !snapshot.Enabled {
			// A disabled contract's discounts are not in effect; projecting
			// them as active would misrepresent prices.
			respondCustomerContractPricingLoadError(c)
			return true
		}
	}
	if snapshot == nil {
		return false
	}
	return respondContractDiscountOverlayPricing(c, snapshot)
}

// respondContractDiscountOverlayPricing serves the native pricing projection
// (accessible models, execution modes, native group ratios) with the bound
// contract's discounts applied on top. It mirrors GetPricing's native shape;
// contracted models carry a per-group effective ratio override
// (native group ratio × contract discount).
func respondContractDiscountOverlayPricing(c *gin.Context, snapshot *model.ContractEntitySnapshot) bool {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" {
		user, loadErr := model.GetUserCache(c.GetInt("id"))
		if loadErr != nil {
			respondCustomerContractPricingLoadError(c)
			return true
		}
		userGroup = user.Group
	}
	groupRatio := map[string]float64{}
	for g, r := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[g] = r
	}
	for g := range groupRatio {
		if special, ok := ratio_setting.GetGroupGroupRatio(userGroup, g); ok {
			groupRatio[g] = special
		}
	}
	usableGroup := service.GetUserUsableGroups(userGroup)
	var pricing []model.Pricing
	batch := c.Query("execution_mode") == "batch"
	if batch {
		var err error
		pricing, err = batchPricingForGroups(usableGroup)
		if err != nil {
			respondCustomerContractPricingLoadError(c)
			return true
		}
	} else {
		pricing = filterPricingByUsableGroups(model.GetPricing(), usableGroup)
	}
	service.AttachPricingBillingDisplay(pricing)
	for g := range groupRatio {
		if _, ok := usableGroup[g]; !ok {
			delete(groupRatio, g)
		}
	}
	usableGroups := make([]string, 0, len(usableGroup))
	for g := range usableGroup {
		usableGroups = append(usableGroups, g)
	}
	views, err := service.ApplyContractDiscountOverlay(pricing, usableGroups, userGroup, snapshot)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	// 只读展示投影：合同折扣由展示层叠加，不并入原生计费数据。
	response := gin.H{
		"success":            true,
		"data":               views,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(userGroup),
		"pricing_version":    "customer-contract",
	}
	if batch {
		response["execution_mode"] = "batch"
		response["auto_groups"] = []string{}
		response["vendors"] = []model.PricingVendor{}
	}
	c.JSON(http.StatusOK, response)
	return true
}
