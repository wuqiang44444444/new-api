package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
)

type seedancePricingModel struct {
	SeedancePublicModel
	billingContractConflict bool
}

type seedancePricingCatalog map[string]seedancePricingModel

func loadSeedancePricingCatalog() (seedancePricingCatalog, error) {
	models, err := GetConfiguredSeedancePublicModels()
	if err != nil {
		return nil, err
	}
	catalog := make(seedancePricingCatalog, len(models))
	if len(models) == 0 {
		return catalog, nil
	}
	nativeModels, err := oppositeSeedancePricingModels(DB, constant.ChannelTypeSeedanceLink, 0)
	if err != nil {
		return nil, err
	}
	for _, item := range models {
		catalog[item.ModelName] = seedancePricingModel{SeedancePublicModel: item, billingContractConflict: nativeModels[item.ModelName]}
	}
	return catalog, nil
}

func (catalog seedancePricingCatalog) mergeGroups(groupsByModel map[string]*types.Set[string]) {
	for modelName, item := range catalog {
		groups, ok := groupsByModel[modelName]
		if !ok {
			groups = types.NewSet[string]()
			groupsByModel[modelName] = groups
		}
		for _, group := range item.Groups {
			groups.Add(group)
		}
	}
}

func (catalog seedancePricingCatalog) mergeEndpoints(endpointsByModel map[string][]string) {
	for modelName, item := range catalog {
		if item.billingContractConflict {
			continue
		}
		endpointsByModel[modelName] = []string{string(constant.EndpointTypeModelArkVideo)}
	}
}

func (catalog seedancePricingCatalog) apply(modelName string, pricing *Pricing) {
	item, ok := catalog[modelName]
	if !ok {
		return
	}
	pricing.BillingContractConflict = item.billingContractConflict
	if item.billingContractConflict {
		pricing.API = nil
		return
	}
	api := item.API
	pricing.API = &api
	pricing.OwnerBy = "new-api"
}

func (catalog seedancePricingCatalog) keepDisabled(modelName string, pricing *Pricing) bool {
	if _, ok := catalog[modelName]; !ok {
		return false
	}
	pricing.Available = false
	pricing.Availability = "disabled"
	return true
}
