package helper_test

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/asyncimage"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRelayPricingPreservesRequestAcrossModelMapping(t *testing.T) {
	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { savedConfig[k] = v; return nil }))
	savedPrices, err := common.Marshal(ratio_setting.GetModelPriceCopy())
	require.NoError(t, err)
	savedGroups := ratio_setting.GroupRatio2JSONString()
	savedGroupOverrides := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(savedPrices)))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(savedGroupOverrides))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"nano-banana-2-image":"ratio","nano-banana-2-lite-image":"ratio"}`,
	}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"nano-banana-2-image":0.0672,"nano-banana-2-lite-image":0.0336}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"image-mapping-test":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))

	for _, tc := range []struct {
		customer, provider, size string
		price                    float64
	}{
		{"nano-banana-2-image", "nano-banana-2", "1K", 0.0672},
		{"nano-banana-2-lite-image", "nano-banana-2-lite", "", 0.0336},
	} {
		for _, order := range []string{"price-before-mapping", "mapping-before-price"} {
			t.Run(tc.customer+"/"+order, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
				common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAsyncImage)
				common.SetContextKey(c, constant.ContextKeyOriginalModel, tc.customer)
				common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{
					ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2,
				})
				mapping, err := common.Marshal(map[string]string{tc.customer: tc.provider})
				require.NoError(t, err)
				c.Set("model_mapping", string(mapping))
				request := &dto.ImageRequest{Model: tc.customer, Prompt: "a cute cat", N: common.GetPointer(uint(1)), Size: tc.size, ResponseFormat: "url"}
				info := &relaycommon.RelayInfo{
					OriginModelName: tc.customer, Request: request, RelayMode: relayconstant.RelayModeImagesGenerations,
					UserGroup: "image-mapping-test", UsingGroup: "image-mapping-test",
				}
				if order == "mapping-before-price" {
					info.InitChannelMeta(c)
					require.NoError(t, helper.ModelMappedHelper(c, info, request))
				}
				beforePricing := *request
				price, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
				require.NoError(t, err)
				assert.Equal(t, beforePricing, *request, "pricing must not rewrite the request used by the adapter")
				assert.Equal(t, tc.customer, info.GetBillingModelName())
				assert.True(t, price.UsePrice)
				assert.Equal(t, tc.price, price.ModelPrice)
				assert.Equal(t, 1.0, price.GroupRatioInfo.GroupRatio)
				if order == "price-before-mapping" {
					info.InitChannelMeta(c)
					require.NoError(t, helper.ModelMappedHelper(c, info, request))
				}
				assert.Equal(t, tc.provider, info.UpstreamModelName)
				assert.Equal(t, tc.provider, request.Model)
				_, err = (&asyncimage.Adaptor{}).ConvertImageRequest(c, info, *request)
				require.NoError(t, err, "mapped requests must remain convertible after pricing")
			})
		}
	}
}
