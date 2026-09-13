package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceBudgetCanBeSavedAfterUsageExpression(t *testing.T) {
	setupBillingAliasOptionDB(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	ch := model.Channel{Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Models: "repeat-price"}
	ch.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3CMCC})
	require.NoError(t, model.DB.Create(&ch).Error)
	expr := `tier("base", u("tokens") * 5 / 1000000)`
	for _, budget := range []float64{300000, 400000} {
		snapshot, err := model.GetModelPricingSnapshot([]string{"repeat-price"})
		require.NoError(t, err)
		require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: "repeat-price", ExpectedVersion: snapshot.Entries[0].Version, Pricing: model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": expr, billing_setting.TaskPreConsumeTokensOption: budget}}}))
		after, err := model.GetModelPricingSnapshot([]string{"repeat-price"})
		require.NoError(t, err)
		assert.Equal(t, budget, after.Entries[0].Configured[billing_setting.TaskPreConsumeTokensOption])
		assert.Equal(t, expr, after.Entries[0].Configured["billing_setting.billing_expr"])
	}
	require.NoError(t, model.UpdateModelPricingOptions(map[string]string{billing_setting.TaskPreConsumeTokensOption: `{"repeat-price":500000}`}))
}

func TestSeedanceRequiredBudgetCannotBeRemoved(t *testing.T) {
	setupBillingAliasOptionDB(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	ch := model.Channel{Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Models: "budget-required,budget-optional"}
	ch.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1})
	require.NoError(t, model.DB.Create(&ch).Error)
	for _, tc := range []struct {
		name, expr string
		required   bool
	}{{"budget-required", `tier("base", u("tokens") / 1000000)`, true}, {"budget-optional", `tier("base", 0.5)`, false}} {
		snap, err := model.GetModelPricingSnapshot([]string{tc.name})
		require.NoError(t, err)
		pricing := model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": tc.expr}
		err = model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: tc.name, ExpectedVersion: snap.Entries[0].Version, Pricing: pricing}})
		if !tc.required {
			require.NoError(t, err)
			continue
		}
		require.ErrorContains(t, err, "upper bound is required")
		pricing[billing_setting.TaskPreConsumeTokensOption] = float64(300000)
		require.NoError(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: tc.name, ExpectedVersion: snap.Entries[0].Version, Pricing: pricing}}))
		require.ErrorContains(t, model.UpdateModelPricingOptions(map[string]string{billing_setting.TaskPreConsumeTokensOption: `{}`}), "upper bound is required")
		after, err := model.GetModelPricingSnapshot([]string{tc.name})
		require.NoError(t, err)
		assert.Equal(t, float64(300000), after.Entries[0].Configured[billing_setting.TaskPreConsumeTokensOption])
		require.ErrorContains(t, model.UpdateModelPricing([]model.ModelPricingChange{{ModelName: tc.name, ExpectedVersion: after.Entries[0].Version, Reset: true}}), "requires tiered_expr")
	}
}
