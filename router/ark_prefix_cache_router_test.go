package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkSessionDefaultUsesExistingChannelAndPrice(t *testing.T) {
	t.Setenv("ARK_PREFIX_CACHE_PILOT", "")
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	user := model.User{Username: "ark-default-router", Group: "default", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "arkdefaultroutertest", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	baseURL := setting.ArkCacheBaseURL
	const alias, unpricedAlias = "seed-ark-default", "seed-ark-unpriced"
	mapping := `{"seed-ark-default":"` + setting.ArkPrefixCacheModelVersion + `","seed-ark-unpriced":"` + setting.ArkPrefixCacheModelVersion + `"}`
	channel := model.Channel{Name: "native-ark", Type: constant.ChannelTypeVolcEngine, Key: "ark-test-credential",
		Status: common.ChannelStatusEnabled, BaseURL: &baseURL, Group: "default", Models: alias + "," + unpricedAlias, ModelMapping: &mapping}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: alias, ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: unpricedAlias, ChannelId: channel.Id, Enabled: true}).Error)
	price, err := common.Marshal(map[string]string{alias: `tier("storage_hourly",u("storage_token_hours")*0.0025/1000000)`})
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.Option{Key: model.ModelStorageHourlyExprOption, Value: string(price)}).Error)

	engine := gin.New()
	engine.Use(middleware.BodyStorageCleanup())
	engine.POST("/v1/responses", middleware.ModelTokenAuthWith(middleware.ArkPrefixCacheTokenAuth()), controller.ArkPrefixCacheResponses, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	engine.POST("/v1/chat/completions", middleware.ModelTokenAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	explicit := `{"model":"` + alias + `","input":"OK","store":true,"expire_at":100,"caching":{"type":"enabled"}}`
	for _, tt := range []struct {
		name, path, body, code string
		status                 int
	}{
		{"stored Session", "/v1/responses", explicit, "invalid_idempotency_key", http.StatusBadRequest},
		{"different route", "/v1/chat/completions", explicit, constant.ErrorCodeContractRequired, http.StatusForbidden},
		{"stored SSE", "/v1/responses", strings.Replace(explicit, `"store":true`, `"store":true,"stream":true`, 1), constant.ErrorCodeContractRequired, http.StatusForbidden},
		{"ordinary Responses", "/v1/responses", `{"model":"` + alias + `","input":"OK"}`, constant.ErrorCodeContractRequired, http.StatusForbidden},
		{"foreign managed ID", "/v1/responses", `{"model":"` + alias + `","input":"OK","store":true,"previous_response_id":"resp_ark_session_foreign","caching":{"type":"enabled"}}`,
			"cache_not_found", http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token.Key)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, tt.status, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), tt.code)
		})
	}
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "false"}))
	unpriced := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(strings.Replace(explicit, alias, unpricedAlias, 1)))
	unpriced.Header.Set("Content-Type", "application/json")
	unpriced.Header.Set("Authorization", "Bearer "+token.Key)
	unpricedResult := httptest.NewRecorder()
	engine.ServeHTTP(unpricedResult, unpriced)
	assert.Equal(t, http.StatusNoContent, unpricedResult.Code, unpricedResult.Body.String())
}

func TestArkPilotRouterKeepsManagedIDsOutOfDistributionWhenDisabled(t *testing.T) {
	t.Setenv("ARK_PREFIX_CACHE_PILOT", "")
	setupRelayRouterTestDB(t)
	user := model.User{Username: "ark-disabled-router", Group: "default", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "arkdisabledroutertest", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	engine := gin.New()
	setTaskPluginProtocolRouterWithArkPilot(engine)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"native-model","previous_response_id":"`+model.ArkSessionCacheIDPrefix+`saved"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token.Key)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "cache_not_found")
}
