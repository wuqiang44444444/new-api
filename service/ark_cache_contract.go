package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func arkCacheContractPriceBasis(pilot *ArkPrefixCachePilot) (model.Pricing, bool) {
	if pilot == nil || (pilot.Mode != "observe" && !pilot.ChargesSessionStorage()) {
		return model.Pricing{}, false
	}
	if billing_setting.GetBillingMode(pilot.Model) == billing_setting.BillingModeTieredExpr {
		expression, configured := billing_setting.GetBillingExpr(pilot.Model)
		return model.Pricing{ModelName: pilot.Model, BasisPriceConfigured: configured,
			BillingMode: billing_setting.BillingModeTieredExpr, BillingExpr: expression}, configured
	}
	if _, perCall := ratio_setting.GetModelPrice(pilot.Model, false); perCall {
		return model.Pricing{}, false
	}
	ratio, configured, _ := ratio_setting.GetModelRatio(pilot.Model)
	return model.Pricing{ModelName: pilot.Model, BasisPriceConfigured: configured, ModelRatio: ratio,
		CompletionRatio: ratio_setting.GetCompletionRatio(pilot.Model)}, configured
}

// AuthorizeArkCacheContract checks a new write against the request's frozen
// contract. The caller has already verified the fixed channel and credential.
func AuthorizeArkCacheContract(c *gin.Context, pilot *ArkPrefixCachePilot, channel *model.Channel, publicModel string) error {
	if ActiveCustomerContract(c) == nil {
		return nil
	}
	fact, err := ResolveCustomerContractRequest(c, publicModel)
	if err != nil {
		return err
	}
	if fact == nil {
		return ErrCustomerContractUnavailable
	}
	if pilot.AllowsApplication(fact.UserId, common.GetContextKeyInt(c, constant.ContextKeyTokenId)) {
		if pin, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned && pin.ChannelId != channel.Id {
			attachContractRejection(c, contractStageSelection, contractReasonChannelNotInScope, publicModel, nil)
			return ErrCustomerContractScope
		}
		for _, rule := range ActiveCustomerContract(c).Rules {
			if rule.PublicModel == publicModel && model.ArkCacheContractRouteAllowed(pilot, channel, rule) {
				setCustomerContractGroup(c, rule.RouteGroup)
				return nil
			}
		}
	}
	attachContractRejection(c, contractStageSelection, contractReasonChannelNotInScope, publicModel, nil)
	return ErrCustomerContractScope
}
