package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"gorm.io/gorm"
)

// seedancePricingAttribution is the request-scoped projection of the typed
// Seedance customer-model facts for one admin pricing snapshot request. It is
// built once per request from a single channel query covering enabled and
// disabled Seedance Link channels; it is a projection of database facts, not a
// persistent judgment table or a cross-request model cache.
type seedancePricingAttribution struct {
	schemas      map[string]map[string]jsplugin.UsageFieldSchema
	linkModels   map[string]bool
	nativeModels map[string]bool
}

// loadSeedancePricingAttribution loads all Seedance Link channels once and
// builds the per-customer-model attribution used by the admin pricing
// snapshot. Attribution follows the same typed facts as the save-validation
// ownership checks: exact customer model, never provider mappings or the
// generic plugin model index.
func loadSeedancePricingAttribution(db *gorm.DB) (*seedancePricingAttribution, error) {
	var channels []Channel
	if err := db.Where("type = ?", constant.ChannelTypeSeedanceLink).Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	attribution := &seedancePricingAttribution{
		schemas:      make(map[string]map[string]jsplugin.UsageFieldSchema),
		linkModels:   make(map[string]bool),
		nativeModels: make(map[string]bool),
	}
	if len(channels) == 0 {
		return attribution, nil
	}
	nativeModels, nativeErr := oppositeSeedancePricingModels(db, constant.ChannelTypeSeedanceLink, 0)
	if nativeErr != nil {
		return nil, nativeErr
	}
	attribution.nativeModels = nativeModels
	for name, selected := range SeedancePricingChannels(channels) {
		attribution.linkModels[name] = true
		schemas := make([]map[string]jsplugin.UsageFieldSchema, 0, len(selected))
		for _, channel := range selected {
			schemas = append(schemas, seedancebilling.UsageFieldsForProtocol(channel.GetOtherSettings().VideoUpstreamProtocol))
		}
		attribution.schemas[name] = seedancebilling.IntersectUsageFields(schemas...)
	}
	return attribution, nil
}

// pricingContractConflict reports a cross-channel same-name pricing conflict:
// the model is configured on both Seedance Link and native channels, so the
// admin pricing sheet and the public pricing API must both block editing and
// saving instead of letting one contract override the other.
func (a *seedancePricingAttribution) pricingContractConflict(modelName string) bool {
	return a.linkModels[modelName] && a.nativeModels[modelName]
}
