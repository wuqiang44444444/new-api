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
	}{
		{"mapped plugin model", "customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, `tier("base", param("_task.duration_seconds") * 67741.935484)`, ""},
		{"direct plugin model", "declared-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", param("_task.duration_seconds") * 2)`, ""},
		{"feicai probe", "customer-video", dto.VideoUpstreamProtocolFeicaiVideosV1, `tier("base", param("_task.duration_seconds") * param("_task.size_multiplier"))`, ""},
		{"foreign protocol field", "customer-video", dto.VideoUpstreamProtocolModelArkV3Volcengine, `tier("base", param("_task.duration_seconds") * param("_task.size_multiplier"))`, "size_multiplier"},
		{"negative price", "customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, `tier("base", -1)`, "non-negative"},
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
			require.NoError(t, model.DB.Create(&channel).Error)
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
