package billing_setting_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageAdminPricesControlSettlementAndDisplay(t *testing.T) {
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	saved := *settings
	savedRatios, savedPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		*settings = saved
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
	})
	const name = "gpt-image-2"
	const custom = `tier("admin", p * 7 + img * 11 + c * 33 + img_cr * 1.5 + cr * 0.5)`
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-image-2":2.5}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	*settings = billing_setting.BillingSetting{
		BillingMode: map[string]string{name: "tiered_expr"},
		BillingExpr: map[string]string{name: custom},
	}
	assert.Equal(t, "tiered_expr", billing_setting.GetBillingMode(name))
	expression, ok := billing_setting.GetBillingExpr(name)
	require.True(t, ok)
	require.Equal(t, custom, expression)
	projection, err := billingexpr.DisplayProjectionFor(expression)
	require.NoError(t, err)
	require.Len(t, projection.Tiers, 1)
	assert.Equal(t, map[string]float64{"p": 7, "img": 11, "c": 33, "img_cr": 1.5, "cr": 0.5}, projection.Tiers[0].UnitPrices)
	cachedImage := 200
	usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 100, PromptTokensDetails: dto.InputTokenDetails{
		CachedTokens: 300, ImageTokens: 600,
		CachedTokensDetails: &dto.CachedTokenDetails{ImageTokens: &cachedImage},
	}}
	// All five categories use the administrator's rates, including cache overlap.
	result, err := billingexpr.ComputeTieredQuota(&billingexpr.BillingSnapshot{
		ExprString: expression, GroupRatio: 1, QuotaPerUnit: 500000,
	}, service.BuildTieredTokenParams(usage, false, billingexpr.UsedVars(expression)))
	require.NoError(t, err)
	assert.Equal(t, 5075, result.ActualQuotaAfterGroup)
	// Explicitly choosing the old mode must also remain authoritative.
	settings.BillingMode[name] = "ratio"
	assert.Equal(t, "ratio", billing_setting.GetBillingMode(name))
}
