package service

import (
	"fmt"

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
	NativeGroupRatio    string                       `json:"native_group_ratio"`
	EffectiveMultiplier string                       `json:"effective_multiplier"`
	SpecialGroupRatio   bool                         `json:"special_group_ratio"`
	Price               CustomerContractPricePreview `json:"price"`
}

type CustomerContractUserRuleView struct {
	Model               string                       `json:"model"`
	Discount            string                       `json:"discount"`
	ChannelDiscount     string                       `json:"channel_discount"`
	EffectiveMultiplier string                       `json:"effective_multiplier"`
	Available           bool                         `json:"available"`
	Price               CustomerContractPricePreview `json:"price"`
}

// ContractEntityAdminView is the admin drawer view of one contract entity.
type ContractEntityAdminView struct {
	Id      int                             `json:"id"`
	Name    string                          `json:"name"`
	Enabled bool                            `json:"enabled"`
	Version int64                           `json:"version"`
	Rules   []CustomerContractAdminRuleView `json:"rules"`
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
// contract entities. Snapshots must already carry refreshed availability.
func buildContractEntityRuleViews(snapshot *model.ContractEntitySnapshot, userGroup string) ([]CustomerContractAdminRuleView, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("contract snapshot is nil")
	}
	pricing := customerContractPricingIndex()
	channelIDs := make([]int, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		channelIDs = append(channelIDs, rule.ChannelId)
	}
	batchChannels, err := model.BatchContractChannelIDs(channelIDs)
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
		available := rule.Available
		if batchChannels[rule.ChannelId] {
			expr, configured := billing_setting.GetBatchBillingExpr(rule.PublicModel)
			price = CustomerContractPricePreview{PriceType: "tiered_multiplier", BillingMode: "batch_expr", BillingExpr: expr}
			available = available && configured
		}
		result = append(result, CustomerContractAdminRuleView{
			ChannelId: rule.ChannelId, Model: rule.PublicModel, RouteGroup: rule.RouteGroup, Discount: discount, Available: available,
			NativeGroupRatio: decimal.NewFromFloat(groupRatio).String(), EffectiveMultiplier: effective.String(),
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
			Version: snapshots[i].Version, Rules: rules,
		})
	}
	return result, nil
}

// BuildContractEntityUserViews builds owner-visible views for contract
// entities. Snapshots must already carry refreshed availability.
func BuildContractEntityUserViews(snapshots []model.ContractEntitySnapshot, userGroup string) ([]ContractEntityUserView, error) {
	adminRules, err := BuildContractEntityAdminViews(snapshots, userGroup)
	if err != nil {
		return nil, err
	}
	result := make([]ContractEntityUserView, 0, len(adminRules))
	for i := range adminRules {
		models := make([]CustomerContractUserRuleView, 0, len(adminRules[i].Rules))
		for _, rule := range adminRules[i].Rules {
			models = append(models, CustomerContractUserRuleView{
				Model: rule.Model, Discount: rule.Discount, ChannelDiscount: rule.NativeGroupRatio,
				EffectiveMultiplier: rule.EffectiveMultiplier, Available: rule.Available, Price: rule.Price,
			})
		}
		result = append(result, ContractEntityUserView{
			Id: adminRules[i].Id, Name: adminRules[i].Name, Enabled: adminRules[i].Enabled,
			Version: adminRules[i].Version, Models: models,
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
