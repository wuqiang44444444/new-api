package controller

import (
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// Explicit Batch scope prevents a same-named synchronous price being presented
// as the Batch price. Contract pricing takes precedence at the router hook.
func respondBatchPricing(c *gin.Context) bool {
	if c.Query("execution_mode") != "batch" {
		return false
	}
	userGroup := ""
	if c.GetInt("id") > 0 {
		user, err := model.GetUserCache(c.GetInt("id"))
		if err != nil {
			respondCustomerContractPricingLoadError(c)
			return true
		}
		userGroup = user.Group
	}
	usable := service.GetUserUsableGroups(userGroup)
	items, err := batchPricingForGroups(usable)
	if err != nil {
		respondCustomerContractPricingLoadError(c)
		return true
	}
	ratios := map[string]float64{}
	for group := range usable {
		ratio := ratio_setting.GetGroupRatio(group)
		if special, ok := ratio_setting.GetGroupGroupRatio(userGroup, group); ok {
			ratio = special
		}
		ratios[group] = ratio
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "vendors": []model.PricingVendor{}, "group_ratio": ratios, "usable_group": usable, "supported_endpoint": model.GetSupportedEndpointMap(), "auto_groups": []string{}, "pricing_version": "azure-batch", "execution_mode": "batch"})
	return true
}

// batchPricingForGroups is the shared Batch base-price projection for native
// and contract views. It never reads ordinary synchronous model prices.
func batchPricingForGroups(usable map[string]string) ([]model.Pricing, error) {
	channels, err := model.BatchPricingChannels()
	if err != nil {
		return nil, err
	}
	groups := map[string]map[string]bool{}
	for _, channel := range channels {
		for _, name := range channel.GetModels() {
			for _, group := range strings.Split(channel.Group, ",") {
				group = strings.TrimSpace(group)
				if _, ok := usable[group]; !ok {
					continue
				}
				if groups[name] == nil {
					groups[name] = map[string]bool{}
				}
				groups[name][group] = true
			}
		}
	}
	items := []model.Pricing{}
	for name, membership := range groups {
		expr, ok := billing_setting.GetBatchBillingExpr(name)
		if !ok {
			continue
		}
		enabled := []string{}
		for group := range membership {
			enabled = append(enabled, group)
		}
		sort.Strings(enabled)
		items = append(items, model.Pricing{ModelName: name, OwnerBy: "new-api", BillingMode: billing_setting.BillingModeTieredExpr, BillingExpr: expr, EnableGroup: enabled, Available: true, Availability: "available"})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModelName < items[j].ModelName })
	return items, nil
}
