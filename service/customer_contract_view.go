package service

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
)

type CustomerContractPricePreview struct {
	PriceType              string                               `json:"price_type"`
	BillingMode            string                               `json:"billing_mode,omitempty"`
	BaseModelPrice         string                               `json:"base_model_price,omitempty"`
	FinalModelPrice        string                               `json:"final_model_price,omitempty"`
	BaseModelRatio         string                               `json:"base_model_ratio,omitempty"`
	FinalModelRatio        string                               `json:"final_model_ratio,omitempty"`
	CompletionRatio        string                               `json:"completion_ratio,omitempty"`
	BaseImageRatio         string                               `json:"base_image_ratio,omitempty"`
	FinalImageRatio        string                               `json:"final_image_ratio,omitempty"`
	CurrentDiscountedPrice string                               `json:"current_discounted_price,omitempty"`
	BillingDisplay         *billingexpr.DisplayProjection       `json:"billing_display,omitempty"`
	BillingExpr            string                               `json:"billing_expr,omitempty"`
	UsageSchema            map[string]jsplugin.UsageFieldSchema `json:"usage_schema,omitempty"`
}

type CustomerContractAdminRuleView struct {
	ChannelId           int                          `json:"channel_id"`
	Model               string                       `json:"model"`
	RouteGroup          string                       `json:"route_group"`
	Discount            string                       `json:"discount"`
	Available           bool                         `json:"available"`
	UnavailableReason   string                       `json:"unavailable_reason,omitempty"`
	NativeGroupRatio    string                       `json:"native_group_ratio"`
	EffectiveMultiplier string                       `json:"effective_multiplier"`
	SpecialGroupRatio   bool                         `json:"special_group_ratio"`
	Price               CustomerContractPricePreview `json:"price"`
}

// CustomerContractUserRuleView is one deduplicated model/discount row of the
// owner-visible contract view. It expresses the agreed contract discount per
// public model and carries availability without internal channel/group facts.
type CustomerContractUserRuleView struct {
	Model        string `json:"model"`
	Discount     string `json:"discount"`
	Availability string `json:"availability,omitempty"`
}

// ContractEntityAdminView is the admin drawer view of one contract entity.
// The optional template provenance is admin-only traceability; the
// owner-visible ContractEntityUserView never carries it.
type ContractEntityAdminView struct {
	Id                    int                             `json:"id"`
	Name                  string                          `json:"name"`
	Enabled               bool                            `json:"enabled"`
	Version               int64                           `json:"version"`
	SourceTemplateId      int                             `json:"source_template_id,omitempty"`
	SourceTemplateVersion int64                           `json:"source_template_version,omitempty"`
	SourceTemplateName    string                          `json:"source_template_name,omitempty"`
	Rules                 []CustomerContractAdminRuleView `json:"rules"`
}

// ContractEntityUserView is the owner-visible view of one contract entity.
type ContractEntityUserView struct {
	Id      int                            `json:"id"`
	Name    string                         `json:"name"`
	Enabled bool                           `json:"enabled"`
	Version int64                          `json:"version"`
	Models  []CustomerContractUserRuleView `json:"models"`
}

// buildContractEntityRuleViews mirrors the legacy admin rule view builder for
// contract entities. Each (model, channel) rule becomes exactly one entry;
// availability is management information only. Snapshots must already carry
// refreshed availability.
func buildContractEntityRuleViews(snapshot *model.ContractEntitySnapshot, userGroup string) ([]CustomerContractAdminRuleView, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("contract snapshot is nil")
	}
	pricing := customerContractPricingIndex()
	channelIDs := make([]int, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		channelIDs = append(channelIDs, rule.ChannelId)
	}
	batchSources, err := model.CustomerContractBatchSourceIDs(channelIDs)
	if err != nil {
		return nil, err
	}
	result := make([]CustomerContractAdminRuleView, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		discount, err := FormatCustomerContractRatio(rule.RatioUnits)
		if err != nil {
			return nil, err
		}
		groupRatio, hasSpecialRatio := ResolveCustomerContractNativeGroupRatio(userGroup, rule.RouteGroup)
		effective := decimal.NewFromFloat(groupRatio).Mul(decimal.RequireFromString(discount))
		price := buildCustomerContractPricePreview(pricing[rule.PublicModel], effective)
		if batchSources[rule.ChannelId] {
			expr, _ := billing_setting.GetBatchBillingExpr(rule.PublicModel)
			price = CustomerContractPricePreview{PriceType: "tiered_multiplier", BillingMode: "batch_expr", BillingExpr: expr}
		}
		result = append(result, CustomerContractAdminRuleView{
			ChannelId: rule.ChannelId, Model: rule.PublicModel, RouteGroup: rule.RouteGroup, Discount: discount, Available: rule.Available,
			UnavailableReason: rule.UnavailableCategory,
			NativeGroupRatio:  decimal.NewFromFloat(groupRatio).String(), EffectiveMultiplier: effective.String(),
			SpecialGroupRatio: hasSpecialRatio,
			Price:             price,
		})
	}
	return result, nil
}

// BuildContractEntityAdminViews builds admin drawer views for contract
// entities. Snapshots must already carry refreshed availability.
func BuildContractEntityAdminViews(snapshots []model.ContractEntitySnapshot, userGroup string) ([]ContractEntityAdminView, error) {
	result := make([]ContractEntityAdminView, 0, len(snapshots))
	for i := range snapshots {
		rules, err := buildContractEntityRuleViews(&snapshots[i], userGroup)
		if err != nil {
			return nil, err
		}
		result = append(result, ContractEntityAdminView{
			Id: snapshots[i].Id, Name: snapshots[i].Name, Enabled: snapshots[i].Enabled,
			Version: snapshots[i].Version, SourceTemplateId: snapshots[i].SourceTemplateId,
			SourceTemplateVersion: snapshots[i].SourceTemplateVersion,
			SourceTemplateName:    snapshots[i].SourceTemplateName,
			Rules:                 rules,
		})
	}
	return result, nil
}

// BuildContractEntityUserViews builds owner-visible views for contract
// entities: one deduplicated row per public model with its contract discount.
// A model with several same-discount channel rules appears exactly once; an
// unavailable model keeps its agreed discount row.
func BuildContractEntityUserViews(snapshots []model.ContractEntitySnapshot) ([]ContractEntityUserView, error) {
	result := make([]ContractEntityUserView, 0, len(snapshots))
	for i := range snapshots {
		discounts, err := ContractDiscountsFromSnapshot(&snapshots[i])
		if err != nil {
			return nil, err
		}
		models := make([]CustomerContractUserRuleView, 0, len(discounts))
		for publicModel, units := range discounts {
			discount, err := FormatCustomerContractRatio(units)
			if err != nil {
				return nil, err
			}
			models = append(models, CustomerContractUserRuleView{Model: publicModel, Discount: discount})
		}
		sort.Slice(models, func(i, j int) bool { return models[i].Model < models[j].Model })
		result = append(result, ContractEntityUserView{
			Id: snapshots[i].Id, Name: snapshots[i].Name, Enabled: snapshots[i].Enabled,
			Version: snapshots[i].Version, Models: models,
		})
	}
	return result, nil
}

func ResolveCustomerContractNativeGroupRatio(userGroup string, routeGroup string) (float64, bool) {
	groupRatio := ratio_setting.GetGroupRatio(routeGroup)
	specialRatio, hasSpecialRatio := ratio_setting.GetGroupGroupRatio(userGroup, routeGroup)
	if hasSpecialRatio {
		return specialRatio, specialRatio != 1
	}
	return groupRatio, false
}

func customerContractPricingIndex() map[string]model.Pricing {
	items := model.GetPricing()
	result := make(map[string]model.Pricing, len(items))
	for _, item := range items {
		result[item.ModelName] = item
	}
	return result
}

func buildCustomerContractPricePreview(pricing model.Pricing, effectiveMultiplier decimal.Decimal) CustomerContractPricePreview {
	preview := CustomerContractPricePreview{}
	billingMode := pricing.BillingMode
	if billingMode == "" {
		billingMode = billing_setting.GetBillingMode(pricing.ModelName)
	}
	if billingMode == billing_setting.BillingModeTieredExpr {
		preview.PriceType = "tiered_multiplier"
		preview.BillingMode = billingMode
		preview.BillingExpr = pricing.BillingExpr
		preview.BillingDisplay, _ = billingexpr.DisplayProjectionFor(pricing.BillingExpr)
		if len(pricing.BillingUsageSchema) > 0 {
			preview.UsageSchema = make(map[string]jsplugin.UsageFieldSchema, len(pricing.BillingUsageSchema))
			for key, field := range pricing.BillingUsageSchema {
				preview.UsageSchema[key] = field
			}
		}
		return preview
	}
	if pricing.QuotaType == 1 {
		preview.PriceType = "model_price"
		preview.BillingMode = "per_call"
		base := decimal.NewFromFloat(pricing.ModelPrice)
		preview.BaseModelPrice = base.String()
		preview.FinalModelPrice = base.Mul(effectiveMultiplier).String()
		preview.CurrentDiscountedPrice = preview.FinalModelPrice
		return preview
	}
	preview.PriceType = "model_ratio"
	preview.BillingMode = "per_token"
	baseModelRatio := decimal.NewFromFloat(pricing.ModelRatio)
	preview.BaseModelRatio = baseModelRatio.String()
	preview.FinalModelRatio = baseModelRatio.Mul(effectiveMultiplier).String()
	preview.CurrentDiscountedPrice = preview.FinalModelRatio
	if pricing.CompletionRatio > 0 {
		preview.CompletionRatio = decimal.NewFromFloat(pricing.CompletionRatio).String()
	}
	if pricing.ImageRatio != nil {
		baseImageRatio := decimal.NewFromFloat(*pricing.ImageRatio)
		preview.BaseImageRatio = baseImageRatio.String()
		preview.FinalImageRatio = baseImageRatio.Mul(effectiveMultiplier).String()
	}
	return preview
}

func BuildCustomerContractPricePreview(modelName string, effectiveMultiplier decimal.Decimal) CustomerContractPricePreview {
	return buildCustomerContractPricePreview(customerContractPricingIndex()[modelName], effectiveMultiplier)
}
