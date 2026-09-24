package model

import (
	"maps"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
)

type seedancePricingModel struct {
	SeedancePublicModel
	billingContractConflict bool
	usageSchema             map[string]jsplugin.UsageFieldSchema
}

type seedancePricingCatalog map[string]seedancePricingModel

func loadSeedancePricingCatalog() (seedancePricingCatalog, error) {
	models, err := GetConfiguredSeedancePublicModels()
	if err != nil {
		return nil, err
	}
	catalog := make(seedancePricingCatalog, len(models))
	nativeModels, err := oppositeSeedancePricingModels(DB, constant.ChannelTypeSeedanceLink, 0)
	if err != nil {
		return nil, err
	}
	usageSchemas, err := loadSeedanceCatalogUsageSchemas()
	if err != nil {
		return nil, err
	}
	for _, item := range models {
		catalog[item.ModelName] = seedancePricingModel{
			SeedancePublicModel:     item,
			billingContractConflict: nativeModels[item.ModelName],
			usageSchema:             usageSchemas[item.ModelName],
		}
	}
	// The MiniMax Link typed models join the same pricing catalog shape with
	// their own usage schema; native ownership conflicts are checked against
	// the same typed/native pricing boundary.
	minimaxModels, err := GetConfiguredMiniMaxPublicModels()
	if err != nil {
		return nil, err
	}
	if len(minimaxModels) > 0 {
		nativeOfMinimax, err := oppositeSeedancePricingModels(DB, constant.ChannelTypeMiniMaxLink, 0)
		if err != nil {
			return nil, err
		}
		for _, item := range minimaxModels {
			if _, exists := catalog[item.ModelName]; exists {
				continue
			}
			catalog[item.ModelName] = seedancePricingModel{
				SeedancePublicModel:     item,
				billingContractConflict: nativeOfMinimax[item.ModelName],
				usageSchema:             StandardVideoBillingFields(constant.ChannelTypeMiniMaxLink, dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1)),
			}
		}
	}
	return catalog, nil
}

// loadSeedanceCatalogUsageSchemas resolves each Seedance customer model's
// declared u() field contract from the same typed channel facts used by save
// validation and the admin pricing snapshot. Enabled channels take precedence
// via SeedancePricingChannels, and protocols retired from the public catalog
// do not contribute fields. The result is a display fact only: it neither
// grants fulfillment eligibility nor participates in runtime routing, and it
// never carries channel IDs, protocols or upstream model names.
func loadSeedanceCatalogUsageSchemas() (map[string]map[string]jsplugin.UsageFieldSchema, error) {
	var channels []Channel
	if err := DB.Where("type = ?", constant.ChannelTypeSeedanceLink).Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	current := channels[:0]
	for i := range channels {
		settings, err := parsedChannelOtherSettings(&channels[i])
		if err != nil {
			return nil, err
		}
		if settings.VideoUpstreamProtocol.IsValid() && dto.ValidateVideoUpstreamProtocol(settings.VideoUpstreamProtocol) != nil {
			continue
		}
		current = append(current, channels[i])
	}
	schemas := make(map[string]map[string]jsplugin.UsageFieldSchema)
	for name, selected := range SeedancePricingChannels(current) {
		list := make([]map[string]jsplugin.UsageFieldSchema, 0, len(selected))
		for _, channel := range selected {
			list = append(list, StandardVideoBillingFields(channel.Type, channel.GetOtherSettings().VideoUpstreamProtocol))
		}
		schemas[name] = seedancebilling.IntersectUsageFields(list...)
	}
	return schemas, nil
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

func (catalog seedancePricingCatalog) apply(modelName string, proj *Pricing) {
	item, ok := catalog[modelName]
	if !ok {
		return
	}
	proj.BillingContractConflict = item.billingContractConflict
	if item.billingContractConflict {
		proj.API = nil
		return
	}
	proj.BillingUsageSchema = cloneUsageSchema(item.usageSchema)
	api := item.API
	proj.API = &api
	proj.OwnerBy = "new-api"
}

// cloneUsageSchema deep-copies the declared field schema so every cached
// pricing entry owns its slices and maps, mirroring the plugin schema copy in
// updatePricing. Shared backing arrays must not leak across cached entries.
func cloneUsageSchema(schema map[string]jsplugin.UsageFieldSchema) map[string]jsplugin.UsageFieldSchema {
	if len(schema) == 0 {
		return nil
	}
	result := make(map[string]jsplugin.UsageFieldSchema, len(schema))
	for key, field := range schema {
		field.Enum = append([]string(nil), field.Enum...)
		field.Description = maps.Clone(field.Description)
		if field.EnumLabels != nil {
			labels := make(map[string]jsplugin.LocalizedText, len(field.EnumLabels))
			for value, label := range field.EnumLabels {
				labels[value] = maps.Clone(label)
			}
			field.EnumLabels = labels
		}
		result[key] = field
	}
	return result
}

func (catalog seedancePricingCatalog) keepDisabled(modelName string, pricing *Pricing) bool {
	if _, ok := catalog[modelName]; !ok {
		return false
	}
	pricing.Available = false
	pricing.Availability = "disabled"
	return true
}
