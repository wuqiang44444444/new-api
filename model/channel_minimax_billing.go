package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

func validateMiniMaxChannelPricing(channel *Channel) error {
	if channel.Type != constant.ChannelTypeMiniMaxLink {
		return nil
	}
	schema := StandardVideoBillingFields(channel.Type, channel.GetOtherSettings().VideoUpstreamProtocol)
	for _, name := range channel.GetModels() {
		name = strings.TrimSpace(name)
		expression, ok := billing_setting.GetBillingExpr(name)
		if !ok || billing_setting.GetBillingMode(name) != billing_setting.BillingModeTieredExpr {
			return common.NewMessage("Set a tiered billing expression for model {{model}}.", map[string]any{"model": name})
		}
		if err := seedancebilling.ValidateTaskExpression(expression, schema); err != nil {
			return common.NewMessage("Billing for model {{model}} is incompatible with this access method. {{reason}}", map[string]any{"model": name, "reason": err.Error()})
		}
	}
	return nil
}
