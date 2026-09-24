package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxPricingSaveAndCatalogWithoutSeedance(t *testing.T) {
	resetPricingEndpointTestTables(t)
	require.NoError(t, DB.AutoMigrate(&TaskPlugin{}, &Option{}))
	require.NoError(t, DB.Where("key = ?", jsplugin.MinimaxPluginKey).Delete(&TaskPlugin{}).Error)
	t.Cleanup(func() { DB.Where("key = ?", jsplugin.MinimaxPluginKey).Delete(&TaskPlugin{}) })
	plugin := TaskPlugin{Key: jsplugin.MinimaxPluginKey, Version: plugins.MinimaxVersion(), APIVersion: 3,
		Source: plugins.MinimaxSource(), Enabled: true, Active: true}
	require.NoError(t, DB.Create(&plugin).Error)
	channel := Channel{Name: "minimax-pricing", Type: constant.ChannelTypeMiniMaxLink, Models: "customer-h3",
		Group: "default", Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(`{"customer-h3":"MiniMax-H3"}`)}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: constant.VideoUpstreamProtocolJdCloudTaskV1})
	require.NoError(t, DB.Create(&channel).Error)

	catalog, err := loadSeedancePricingCatalog()
	require.NoError(t, err)
	require.Contains(t, catalog, "customer-h3", "a MiniMax-only installation must publish its pricing contract")
	assert.NotContains(t, catalog["customer-h3"].usageSchema, "tokens")
	snapshot, err := GetModelPricingSnapshot([]string{"customer-h3"})
	require.NoError(t, err)
	require.Len(t, snapshot.Entries, 1)
	entry := snapshot.Entries[0]
	assert.False(t, entry.PreconsumeTokenBudget)
	assert.Equal(t, []string{"768p", "2k"}, entry.UsageSchema["resolution"].Enum)
	assert.Equal(t, []string{"16:9", "21:9", "4:3", "1:1", "3:4", "9:16", "adaptive"}, entry.UsageSchema["ratio"].Enum)
	assert.NotContains(t, entry.UsageSchema, "tokens")

	for _, status := range []int{common.ChannelStatusEnabled, common.ChannelStatusManuallyDisabled} {
		require.NoError(t, DB.Model(&channel).Update("status", status).Error)
		values := PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("seconds", u("duration_seconds") * 0.25)`}
		require.NoError(t, ValidateModelPricing("customer-h3", values), "host seconds need no token budget")
		for _, expression := range []string{`tier("tokens", u("tokens"))`, `tier("credit", u("video_output"))`, `tier("legacy", c)`} {
			values["billing_setting.billing_expr"] = expression
			require.Error(t, ValidateModelPricing("customer-h3", values), expression)
		}
	}
}
