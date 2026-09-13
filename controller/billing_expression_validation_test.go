package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionSeedanceBillingUsesConfiguredProtocolBeforePluginLookup(t *testing.T) {
	const pluginKey = "seedance-billing-collision"
	_, err := jsplugin.DefaultRegistry.Register(`
export const meta = {
 apiVersion: 1, key: "seedance-billing-collision", name: "Billing Collision", version: "1.0.0", author: {name: "Test"},
 models: ["declared-video"], fetchMode: "per_task",
 usageSchema: {seconds: {type: "number", unit: "second"}}
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
`, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister(pluginKey) })

	for _, tc := range []struct {
		name, customerModel   string
		protocol              dto.VideoUpstreamProtocol
		expression, errorText string
		disabled              bool
	}{
		{"mapped plugin model with a seedance u() expression", "customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, `tier("base", u("tokens") * 5 / 1000000)`, "", false},
		{"direct plugin model with a seedance u() expression", "declared-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", u("tokens") * 5 / 1000000)`, "", false},
		{"protocol extra field accepted on its own protocol", "customer-video", dto.VideoUpstreamProtocolFeicaiVideosV1, `u("ratio") == "21:9" ? tier("wide", 0.8) : tier("base", 0.4)`, "", false},
		{"protocol extra field rejected on other protocol", "customer-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `u("ratio") == "21:9" ? tier("wide", 0.8) : tier("base", 0.4)`, "ratio", false},
		{"changed legacy probe expression must convert to u()", "customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, `tier("base", param("_task.duration_seconds") * 67741.935484)`, "declared u() fields", false},
		{"negative price", "customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, `tier("base", -1)`, "non-negative", false},
		{"disabled direct plugin model", "declared-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", u("tokens") * 5 / 1000000)`, "", true},
		{"disabled plugin expression rejected", "declared-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", u("seconds") * 2)`, "not declared by the Seedance field contract", true},
		{"plugin null branch rejected", "declared-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", u("seconds") == nil ? 0.0 : u("seconds") * 2)`, "not declared by the Seedance field contract", false},
		{"frozen task param rejected", "declared-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", param("_task.duration_seconds") * 2)`, "declared u() fields", false},
	} {

		t.Run(tc.name, func(t *testing.T) {
			setupBillingAliasOptionDB(t)
			saved := map[string]string{}
			require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
			t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{}`}))
			mapping, err := common.Marshal(map[string]string{tc.customerModel: "declared-video"})
			require.NoError(t, err)
			channel := model.Channel{
				Type: constant.ChannelTypeSeedanceLink, Key: "fixture-key", Status: common.ChannelStatusEnabled,
				Name: "billing-fixture", Models: tc.customerModel, Group: "default", ModelMapping: common.GetPointer(string(mapping)),
			}
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: tc.protocol})
			if tc.disabled {
				channel.Status = common.ChannelStatusManuallyDisabled
			}
			require.NoError(t, model.DB.Create(&channel).Error)
			for key, value := range map[string]any{
				"billing_setting.billing_mode":           "tiered_expr",
				"task_billing_setting.preconsume_tokens": 300000,
			} {
				encoded, err := common.Marshal(map[string]any{tc.customerModel: value})
				require.NoError(t, err)
				require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: string(encoded)}).Error)
			}
			model.InitChannelCache()
			generation := jsplugin.DefaultRegistry.Generation()
			if tc.customerModel == "declared-video" {
				_, ok := generation.GetByModel(tc.customerModel)
				require.True(t, ok)
			} else {
				_, ok := model.ResolveTaskModelAlias(generation, tc.customerModel)
				require.True(t, ok, "fixture must reproduce native plugin alias collision")
			}
			expressions, err := common.Marshal(map[string]string{tc.customerModel: tc.expression})
			require.NoError(t, err)
			body, err := common.Marshal(OptionUpdateRequest{Key: "billing_setting.billing_expr", Value: string(expressions)})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
			UpdateOption(c)
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, tc.errorText == "", response.Success, response.Message)
			if tc.errorText != "" {
				assert.Contains(t, response.Message, tc.errorText)
				var count int64
				require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "billing_setting.billing_expr").Count(&count).Error)
				assert.Zero(t, count, "invalid expression must not be saved")
			} else {
				var option model.Option
				require.NoError(t, model.DB.Where("key = ?", "billing_setting.billing_expr").First(&option).Error)
				assert.JSONEq(t, string(expressions), option.Value)
			}
		})
	}
}

func TestSeedancePricingConflictRejectsAllPricingOptionWrites(t *testing.T) {
	for _, key := range []string{"billing_setting.billing_expr", "billing_setting.billing_mode", "task_billing_setting.preconsume_tokens", "ModelPrice", "ModelRatio", "ImageRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "AudioRatio", "AudioCompletionRatio"} {
		t.Run(key, func(t *testing.T) {
			setupBillingAliasOptionDB(t)
			for _, channelType := range []int{constant.ChannelTypeSeedanceLink, constant.ChannelTypeDoubaoVideo} {
				require.NoError(t, model.DB.Create(&model.Channel{Type: channelType, Status: common.ChannelStatusManuallyDisabled, Models: "shared-price"}).Error)
			}
			require.NoError(t, model.DB.Create(&model.Option{Key: key, Value: `{}`}).Error)
			var value any = 250000
			if key == "billing_setting.billing_expr" {
				value = `tier("base", u("tokens") * 5 / 1000000)`
			} else if key == "billing_setting.billing_mode" {
				value = "tiered_expr"
			}
			values, err := common.Marshal(map[string]any{"shared-price": value})
			require.NoError(t, err)
			body, err := common.Marshal(OptionUpdateRequest{Key: key, Value: string(values)})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
			UpdateOption(c)
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, "distinct customer model names")
			var stored model.Option
			require.NoError(t, model.DB.Where("key = ?", key).First(&stored).Error)
			assert.JSONEq(t, `{}`, stored.Value)
		})
	}
}

func TestSeedancePriceValidationPrefersActiveProtocolButChecksAllInactiveContracts(t *testing.T) {
	for _, active := range []bool{true, false} {
		name := "inactive contracts"
		if active {
			name = "active contract"
		}
		t.Run(name, func(t *testing.T) {
			setupBillingAliasOptionDB(t)
			for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolFeicaiVideosV1} {
				channel := model.Channel{Type: constant.ChannelTypeSeedanceLink, Models: "shared-price", Status: common.ChannelStatusManuallyDisabled}
				if active && protocol == dto.VideoUpstreamProtocolFeicaiVideosV1 {
					channel.Status = common.ChannelStatusEnabled
				}
				channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol})
				require.NoError(t, model.DB.Create(&channel).Error)
			}
			handled, err := model.ValidateSeedanceBillingExpression("shared-price", `u("ratio") == "21:9" ? tier("wide", 0.8) : tier("base", 0.4)`)
			assert.True(t, handled)
			if active {
				require.NoError(t, err, "an inactive channel must not constrain the active protocol")
			} else {
				require.ErrorContains(t, err, "ratio", "all inactive contracts must accept a shared price")
			}
		})
	}
}

// Current Seedance prices cannot retain the legacy input contract, even unchanged.
func TestSeedanceUnchangedLegacyExpressionIsRejected(t *testing.T) {
	setupBillingAliasOptionDB(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	const legacy = `tier("base", param("_task.duration_seconds") * 67741.935484)`
	legacyMap, mapErr := common.Marshal(map[string]string{"customer-video": legacy})
	require.NoError(t, mapErr)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": string(legacyMap)}))

	channel := model.Channel{
		Type: constant.ChannelTypeSeedanceLink, Key: "fixture-key", Status: common.ChannelStatusEnabled,
		Name: "billing-fixture", Models: "customer-video", Group: "default",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingModelArkV1})
	require.NoError(t, model.DB.Create(&channel).Error)
	model.InitChannelCache()

	expressions, err := common.Marshal(map[string]string{"customer-video": legacy})
	require.NoError(t, err)
	body, err := common.Marshal(OptionUpdateRequest{Key: "billing_setting.billing_expr", Value: string(expressions)})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
	UpdateOption(c)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "declared u() fields")
}
