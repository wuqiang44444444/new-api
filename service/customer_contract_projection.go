package service

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"sort"
)

func selectCustomerContractBatchChannel(c *gin.Context, group, publicModel string) (*model.Channel, error) {
	if ActiveCustomerContract(c) == nil {
		return model.SelectEnabledBatchChannel(group, publicModel)
	}
	pinID := 0
	if pin, ok, _ := GetChannelConstraints(c).ResolvedPin(); ok {
		pinID = pin.ChannelId
	}
	channel, _, err := CustomerContractTypedChannel(c, publicModel, constant.ChannelTypeAzureBatch, pinID)
	return channel, err
}

// Public model/group projections never expose channel identity. Sources must
// match the execution mode; a Batch-only contract cannot advertise sync prices.
func ProjectCustomerContractPricing(c *gin.Context, pricing []model.Pricing, userGroup string, snapshot *model.ContractEntitySnapshot, batch bool) ([]CustomerContractPricingView, error) {
	rules, err := EffectiveContractRules(snapshot, userGroup)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ChannelId)
	}
	batchIDs, err := model.CustomerContractBatchSourceIDs(ids)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]map[string]bool)
	var selectedRules []model.ContractEntityRule
	for _, rule := range rules {
		if batchIDs[rule.ChannelId] != batch || !ContractTokenModelAllowed(c, rule.PublicModel) {
			continue
		}
		selectedRules = append(selectedRules, rule)
		if groups[rule.PublicModel] == nil {
			groups[rule.PublicModel] = make(map[string]bool)
		}
		groups[rule.PublicModel][rule.RouteGroup] = true
	}
	metadata, err := model.GetContractModelMetadata(selectedRules)
	if err != nil {
		return nil, err
	}
	views := make([]CustomerContractPricingView, 0, len(groups))
	for _, item := range pricing {
		allowed := groups[item.ModelName]
		if len(allowed) == 0 {
			continue
		}
		if !batch {
			item.API = metadata[item.ModelName].API
			item.SupportedEndpointTypes = metadata[item.ModelName].SupportedEndpointTypes
		}
		item.EnableGroup = nil
		for group := range allowed {
			item.EnableGroup = append(item.EnableGroup, group)
		}
		sort.Strings(item.EnableGroup)
		items, err := ApplyContractDiscountOverlay([]model.Pricing{item}, item.EnableGroup, userGroup, snapshot)
		if err != nil {
			return nil, err
		}
		views = append(views, items...)
	}
	return views, nil
}
