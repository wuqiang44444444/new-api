package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRelayBillingUsesValidatedCountBeforeChannelInitialization(t *testing.T) {
	previous := ratio_setting.GetModelPriceCopy()
	encoded, err := common.Marshal(previous)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(encoded))) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"client-image":0.035}`))
	for _, count := range []int{0, 1, 2} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAsyncImage)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "client-image")
		common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
		c.Set("model_mapping", `{"client-image":"doubao-seedream-5-0-260128"}`)
		request := &dto.ImageRequest{Model: "client-image", Prompt: "edit image"}
		info := &relaycommon.RelayInfo{Request: request, OriginModelName: "client-image", RelayMode: relayconstant.RelayModeImagesGenerations}
		if count > 0 {
			info.RelayMode = relayconstant.RelayModeImagesEdits
			urls := make([]string, count)
			for i := range urls {
				urls[i] = "https://example.com/input.png"
			}
			var err error
			request.Images, err = common.Marshal(urls)
			require.NoError(t, err)
		}
		require.NoError(t, prepareImageRelayBilling(c, info))
		require.NotNil(t, info.BillingRequestInput)
		assert.NotContains(t, string(info.BillingRequestInput.Body), "https://")
		cost, _, err := billingexpr.RunExprWithRequest(`tier("2K", (0.60 + (param("input_image_count") == nil ? 0 : max(param("input_image_count") - 1, 0) * 0.02)) / 7 * 1000000)`, billingexpr.TokenParams{}, *info.BillingRequestInput)
		require.NoError(t, err)
		want := 0.60
		if count == 2 {
			want = 0.62
		}
		assert.InDelta(t, want, cost/1000000*7, 1e-9)
		assert.Nil(t, info.ChannelMeta, "pricing must not mutate the channel execution metadata")
	}
}

func TestImageRelayRejectsTokenPricingBeforeSending(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"local-nano":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"local-nano":"p * 0.5 + img_o * 60"}`,
	}))
	for _, tc := range []struct {
		name     string
		protocol dto.ImageUpstreamProtocol
		model    string
	}{
		{"FunCloud", dto.ImageUpstreamProtocolFunCloudAIGCV2, "nano-banana-2"},
		{"FunCloud lite", dto.ImageUpstreamProtocolFunCloudAIGCV2, "nano-banana-2-lite"},
		{"Moxing partial usage", dto.ImageUpstreamProtocolMoxingImagesV1, "doubao-seedream-5-0-260128"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAsyncImage)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "local-nano")
			common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{ImageUpstreamProtocol: tc.protocol})
			mapping, err := common.Marshal(map[string]string{"local-nano": tc.model})
			require.NoError(t, err)
			c.Set("model_mapping", string(mapping))
			info := &relaycommon.RelayInfo{OriginModelName: "local-nano", Request: &dto.ImageRequest{Model: "local-nano", Prompt: "red cup"}}
			err = prepareImageRelayBilling(c, info)
			require.EqualError(t, err, "the current image adapter cannot supply the verified usage required by this billing expression")
			assert.Nil(t, info.BillingRequestInput)
			assert.NotContains(t, err.Error(), string(tc.protocol), "public errors must not expose the selected upstream protocol")
		})
	}
}

func TestImageRelayPricingContract(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	prices, err := common.Marshal(ratio_setting.GetModelPriceCopy())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices))) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"local-pro":0.1}`))
	for _, tc := range []struct {
		name, expression string
		count            int
		allowed          bool
	}{
		{"fixed single", "", 1, true}, {"fixed multiple", "", 2, false},
		{"input expression", `tier("base", 10 + param("input_image_count"))`, 2, true},
		{"usage access", `u("output_tokens") == nil ? 0 : u("output_tokens")`, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode := "ratio"
			if tc.expression != "" {
				mode = "tiered_expr"
			}
			modes, err := common.Marshal(map[string]string{"local-pro": mode})
			require.NoError(t, err)
			exprs, err := common.Marshal(map[string]string{"local-pro": tc.expression})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": string(modes), "billing_setting.billing_expr": string(exprs)}))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAsyncImage)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "local-pro")
			common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
			c.Set("model_mapping", `{"local-pro":"doubao-seedream-5-0-pro-260628"}`)
			refs := make([]string, tc.count)
			for i := range refs {
				refs[i] = "https://example.com/input.png"
			}
			raw, err := common.Marshal(refs)
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{OriginModelName: "local-pro", RelayMode: relayconstant.RelayModeImagesEdits, Request: &dto.ImageRequest{Model: "local-pro", Prompt: "edit", Images: raw}}
			err = prepareImageRelayBilling(c, info)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Nil(t, info.BillingRequestInput)
			}
		})
	}
}
