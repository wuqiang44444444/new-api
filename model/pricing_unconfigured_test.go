package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingDistinguishesMissingRatioFromExplicitDefault(t *testing.T) {
	resetPricingEndpointTestTables(t)
	original := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(original)) })
	// 显式配置恰好等于缺价默认倍率 37.5,必须与缺价区分。
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"explicit-37-5":37.5}`))
	ch := Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "ratio-src", Models: "explicit-37-5,unset-price-model", Group: "default"}
	require.NoError(t, DB.Create(&ch).Error)
	insertPricingEndpointAbility(t, ch.Id, "explicit-37-5")
	insertPricingEndpointAbility(t, ch.Id, "unset-price-model")
	InitChannelCache()
	InvalidatePricingCache()

	prices := pricingByModel(GetPricing())
	require.Contains(t, prices, "explicit-37-5")
	require.Contains(t, prices, "unset-price-model")
	assert.True(t, prices["explicit-37-5"].BasisPriceConfigured)
	assert.InDelta(t, 37.5, prices["explicit-37-5"].ModelRatio, 1e-12)
	assert.False(t, prices["unset-price-model"].BasisPriceConfigured)
	assert.InDelta(t, 37.5, prices["unset-price-model"].ModelRatio, 1e-12)
	encoded, err := common.Marshal(prices["unset-price-model"])
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"basis_price_configured":false`)
}
