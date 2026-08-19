package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
)

type CustomerContractPricePreview struct {
	PriceType              string `json:"price_type"`
	BillingMode            string `json:"billing_mode,omitempty"`
	BaseModelPrice         string `json:"base_model_price,omitempty"`
	FinalModelPrice        string `json:"final_model_price,omitempty"`
	BaseModelRatio         string `json:"base_model_ratio,omitempty"`
	FinalModelRatio        string `json:"final_model_ratio,omitempty"`
	CompletionRatio        string `json:"completion_ratio,omitempty"`
	BaseImageRatio         string `json:"base_image_ratio,omitempty"`
	FinalImageRatio        string `json:"final_image_ratio,omitempty"`
	CurrentDiscountedPrice string `json:"current_discounted_price,omitempty"`
}

type CustomerContractAdminRuleView struct {
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

func BuildCustomerContractAdminRules(snapshot *model.CustomerContractSnapshot, userGroup string) ([]CustomerContractAdminRuleView, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("contract snapshot is nil")
	}
	if err := RefreshCustomerContractAvailability(snapshot); err != nil {
		return nil, err
	}
	pricing := customerContractPricingIndex()
	result := make([]CustomerContractAdminRuleView, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		discount, err := FormatCustomerContractRatio(rule.RatioUnits)
		if err != nil {
			return nil, err
		}
		groupRatio, hasSpecialRatio := ResolveCustomerContractNativeGroupRatio(userGroup, rule.RouteGroup)
		effective := decimal.NewFromFloat(groupRatio).Mul(decimal.RequireFromString(discount))
		result = append(result, CustomerContractAdminRuleView{
			Model: rule.PublicModel, RouteGroup: rule.RouteGroup, Discount: discount, Available: rule.Available,
			NativeGroupRatio: decimal.NewFromFloat(groupRatio).String(), EffectiveMultiplier: effective.String(),
			SpecialGroupRatio: hasSpecialRatio,
			Price:             buildCustomerContractPricePreview(pricing[rule.PublicModel], effective),
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

func BuildCustomerContractUserRules(snapshot *model.CustomerContractSnapshot, userGroup string) ([]CustomerContractUserRuleView, error) {
	adminRules, err := BuildCustomerContractAdminRules(snapshot, userGroup)
	if err != nil {
		return nil, err
	}
	result := make([]CustomerContractUserRuleView, 0, len(adminRules))
	for _, rule := range adminRules {
		result = append(result, CustomerContractUserRuleView{
			Model: rule.Model, Discount: rule.Discount, ChannelDiscount: rule.NativeGroupRatio,
			EffectiveMultiplier: rule.EffectiveMultiplier, Available: rule.Available, Price: rule.Price,
		})
	}
	return result, nil
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
