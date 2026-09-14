package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedancePricingKeepsOwnBillingContractAcrossPluginNameAndAliasCollisions(t *testing.T) {
	resetPricingEndpointTestTables(t)
	seedPublishedSeedanceTestArtifact(t)
	t.Cleanup(func() { require.NoError(t, DB.Where("key = ?", "seedance-link").Delete(&TaskPlugin{}).Error) })
	const canonical = "doubao-seedance-2-0-fast-260128"
	_, err := jsplugin.DefaultRegistry.Register(`
export const meta = {apiVersion:1,key:"seedance-pricing-boundary",name:"Boundary",version:"1.0.0",author:{name:"Test"},
models:["doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-260128"],fetchMode:"per_task",
usageSchema:{tokens:{type:"number",unit:"token"}},usageExamples:[{label:"Sample",facts:{tokens:100000}}]};
export function buildSubmitRequest(){return {};}
export function parseSubmitResponse(){return {};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {};}
`, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("seedance-pricing-boundary") })
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"doubao-seedance-2-0-fast-260128":"tiered_expr","link-priced":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"doubao-seedance-2-0-fast-260128":"u(\"tokens\") * 5 / 1000000","link-priced":"tier(\"base\", c * 5)"}`,
	}))
	for _, fixture := range []struct {
		name   string
		status int
	}{
		{"doubao-seedance-2-0-260128", common.ChannelStatusManuallyDisabled},
		{"link-priced", common.ChannelStatusEnabled},
		{"link-unpriced", common.ChannelStatusEnabled},
		{"link-disabled", common.ChannelStatusManuallyDisabled},
	} {
		mapping, err := common.Marshal(map[string]string{fixture.name: canonical})
		require.NoError(t, err)
		ch := Channel{Type: constant.ChannelTypeSeedanceLink, Status: fixture.status, Name: fixture.name, Models: fixture.name, Group: "default", ModelMapping: common.GetPointer(string(mapping))}
		ch.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
		require.NoError(t, DB.Create(&ch).Error)
	}
	mapping := `{"native-alias":"doubao-seedance-2-0-fast-260128"}`
	native := Channel{Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Name: "native", Models: canonical + ",native-alias", Group: "default", ModelMapping: &mapping}
	require.NoError(t, DB.Create(&native).Error)
	insertPricingEndpointAbility(t, native.Id, canonical)
	insertPricingEndpointAbility(t, native.Id, "native-alias")
	InitChannelCache()
	InvalidatePricingCache()
	prices := pricingByModel(GetPricing())
	for _, name := range []string{"doubao-seedance-2-0-260128", "link-priced", "link-unpriced", "link-disabled"} {
		require.Contains(t, prices, name)
		// 同源字段合同:Seedance 模型从类型化渠道事实取得自己的用量字段,
		// 不再依赖通用插件模型索引,也不继承插件示例。
		schema := prices[name].BillingUsageSchema
		require.NotEmpty(t, schema, name)
		assert.Equal(t, "token", schema["tokens"].Unit, name)
		assert.Contains(t, schema, "duration_seconds", name)
		assert.Contains(t, schema, "resolution", name)
		assert.Empty(t, prices[name].BillingUsageExamples, name)
		if name == "link-priced" {
			assert.Equal(t, `tier("base", c * 5)`, prices[name].BillingExpr)
		} else {
			assert.Empty(t, prices[name].BillingExpr, name)
		}
	}
	for _, name := range []string{canonical, "native-alias"} {
		assert.NotEmpty(t, prices[name].BillingUsageSchema, name)
		assert.NotEmpty(t, prices[name].BillingUsageExamples, name)
		assert.Equal(t, `u("tokens") * 5 / 1000000`, prices[name].BillingExpr, name)
	}
	// A pre-existing cross-type collision must be reported, never silently
	// presented as a Link API or a native usage contract.
	legacyNative := Channel{Type: constant.ChannelTypeDoubaoVideo, Status: common.ChannelStatusEnabled, Models: "link-disabled", Group: "default"}
	require.NoError(t, DB.Create(&legacyNative).Error)
	insertPricingEndpointAbility(t, legacyNative.Id, "link-disabled")
	InvalidatePricingCache()
	prices = pricingByModel(GetPricing())
	assert.True(t, prices["link-disabled"].BillingContractConflict)
	assert.Nil(t, prices["link-disabled"].API)
	assert.Empty(t, prices["link-disabled"].BillingUsageSchema)
	assert.False(t, prices[canonical].BillingContractConflict)
	assert.NotEmpty(t, prices[canonical].BillingUsageSchema)
}
