package router

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAPIKeyContractPolicyRelayRoutesRequireKey(t *testing.T) {
	setupRelayRouterTestDB(t)
	engine := gin.New()
	SetRelayRouter(engine)
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	dynamicSegment := regexp.MustCompile(`[:*][^/]+`)
	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/v1/") {
			continue
		}
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			request := httptest.NewRequest(route.Method, dynamicSegment.ReplaceAllString(route.Path, "contract-test"), nil)
			result := httptest.NewRecorder()
			engine.ServeHTTP(result, request)
			assert.Equal(t, http.StatusUnauthorized, result.Code, result.Body.String())
		})
	}
}

func TestAPIKeyContractPolicyOnPublishedRoutes(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	user := model.User{Username: "contract-policy", Group: "partner", Status: common.UserStatusEnabled, Quota: 100, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "contractpolicytest", Group: "retired-group", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	engine := gin.New()
	errors := logtest.New(t)
	engine.Use(errors.Middleware())
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	SetAssetRouter(engine)
	SetTaskRouter(engine)
	var directoryQueries int
	queries := model.DB.Callback().Query()
	require.NoError(t, queries.Before("gorm:query").Register("contract_policy_directory", func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "channels", "abilities", "models":
			directoryQueries++
		}
	}))
	t.Cleanup(func() { require.NoError(t, queries.Remove("contract_policy_directory")) })
	call := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(`{"task_id":"absent"}`))
		request.Header.Set("Authorization", "Bearer "+token.Key)
		request.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	for _, tc := range []struct {
		path, response string
		headers        map[string]string
	}{
		{"/v1/models", `{"success":true,"data":[],"object":"list"}`, nil},
		{"/v1/models", `{"data":[],"first_id":"","last_id":"","has_more":false}`, map[string]string{"x-api-key": token.Key, "anthropic-version": "2023-06-01"}},
		{"/v1beta/models", `{"models":[],"nextPageToken":null}`, map[string]string{"x-goog-api-key": token.Key}},
		{"/v1beta/openai/models", `{"success":true,"data":[],"object":"list"}`, nil},
		{"/api/v3/contents/generations/models", `{"success":true,"data":[],"object":"list"}`, nil},
	} {
		t.Run(tc.path+tc.response, func(t *testing.T) {
			result := call(http.MethodGet, tc.path, tc.headers)
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			assert.JSONEq(t, tc.response, result.Body.String())
		})
	}
	detail := call(http.MethodGet, "/v1/models/gpt-4o", nil)
	unknown := call(http.MethodGet, "/v1/models/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, detail.Code)
	assert.Equal(t, http.StatusNotFound, unknown.Code)
	assert.JSONEq(t, detail.Body.String(), unknown.Body.String())
	assert.Zero(t, directoryQueries, "restricted discovery must return before group and configured video model queries")
	for _, tc := range []struct{ method, path, errorField string }{
		{http.MethodGet, "/v1/responses", `"code":"contract_required"`},
		{http.MethodGet, "/v1/realtime?model=gpt-4o", `"code":"contract_required"`},
		{http.MethodPost, "/v1/chat/completions", `"code":"contract_required"`},
		{http.MethodPost, "/v1/messages", `"type":"permission_error"`},
		{http.MethodPost, "/v1beta/models/gemini:generateContent", `"code":"contract_required"`},
		{http.MethodPost, "/v1/responses", `"code":"contract_required"`},
		{http.MethodPost, "/v1/images/generations", `"code":"contract_required"`},
		{http.MethodPost, "/v1/video/generations", `"code":"contract_required"`},
		{http.MethodPost, "/v1/videos/old/remix", `"code":"contract_required"`},
		{http.MethodPost, "/api/v3/contents/generations/tasks", `"code":"contract_required"`},
		{http.MethodPost, "/kling/v1/videos/text2video", `"code":1103`},
		{http.MethodPost, "/jimeng/?Action=CVSync2AsyncSubmitTask", `"code":50400`},
		{http.MethodPost, "/mj/submit/imagine", `"description":`},
		{http.MethodPost, "/fast/mj/submit/imagine", `"description":`},
		{http.MethodGet, "/v1/assets/opaque?model=gpt-4o", `"type":"asset_error"`},
		{http.MethodPost, "/v1/batches", `"success":false`},
		{http.MethodPost, "/v1/tasks/example", `"code":"contract_required"`},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			headers := map[string]string{"Idempotency-Key": "contract-policy-test"}
			if strings.Contains(tc.path, "/mj/") {
				headers["Authorization"] = "midjourney-proxy"
				headers["mj-api-secret"] = token.Key
			}
			result := call(tc.method, tc.path, headers)
			assert.Equal(t, http.StatusForbidden, result.Code, result.Body.String())
			assert.Contains(t, result.Body.String(), tc.errorField)
		})
	}
	var claims int64
	require.NoError(t, model.DB.Model(&model.TaskCreateIdempotency{}).Count(&claims).Error)
	assert.Zero(t, claims)
	assert.Contains(t, errors.String(), "stage=contract_scope reason=contract_required public_code=contract_required")
	for _, result := range []*httptest.ResponseRecorder{
		call(http.MethodGet, "/v1/tasks/absent", nil),
		call(http.MethodPost, "/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", nil),
	} {
		assert.Equal(t, http.StatusNotFound, result.Code, result.Body.String())
	}
	require.NoError(t, model.DB.Model(&token).Update("group", "").Error)
	file := call(http.MethodPost, "/v1/files", nil)
	assert.Equal(t, http.StatusBadRequest, file.Code, file.Body.String())
	assert.Contains(t, file.Body.String(), "multipart file field")
}

func TestAPIKeyContractPolicyOnNativePluginRoutes(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	user := model.User{Username: "native-contract-policy", Group: "default", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "nativecontractpolicy", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	engine := gin.New()
	errors := logtest.New(t)
	engine.Use(errors.Middleware())
	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("vidu")
	require.NoError(t, err)
	_, err = registry.RegisterFactory(source, jsplugin.Options{Key: "vidu"})
	require.NoError(t, err)
	require.NoError(t, registry.SetGenerationPreparer(newPluginGenerationBuilder(engine.Routes(), nil, productionPluginRouteHandlers).prepare))
	require.Empty(t, registry.RoutingErrors())
	engine.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)
	request := httptest.NewRequest(http.MethodPost, "/ent/v2/text2video", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Token "+token.Key)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), i18n.Translate("en", i18n.MsgAPIKeyContractRequired))
	assert.Contains(t, errors.String(), "stage=contract_scope reason=contract_required")
}

func TestAPIKeyContractPolicyPreservesNativePluginHistory(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	user := model.User{Username: "native-history", Group: "default", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "nativehistorykey", Group: "retired", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	task := model.Task{UserId: user.Id, AppID: token.Id, TaskID: "suno-history", Platform: constant.TaskPlatform("sunoapi"), Status: model.TaskStatusQueued}
	require.NoError(t, model.DB.Create(&task).Error)
	engine := gin.New()
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"sunoapi", "vertex-ai", "alibaba", "doubao"} {
		source, err := plugins.Source(key)
		require.NoError(t, err)
		_, err = registry.RegisterFactory(source, jsplugin.Options{Key: key})
		require.NoError(t, err)
	}
	require.NoError(t, registry.SetGenerationPreparer(newPluginGenerationBuilder(engine.Routes(), nil, productionPluginRouteHandlers).prepare))
	require.Empty(t, registry.RoutingErrors())
	engine.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)
	for _, operation := range []struct {
		name, method, path, body, response string
		status                             int
	}{
		{"dynamic history", http.MethodPost, "/suno/fetch", `{"ids":["suno-history"]}`, "suno-history", http.StatusOK},
		{"static history", http.MethodGet, "/suno/fetch/suno-history", "", "suno-history", http.StatusOK},
		{"dynamic submit", http.MethodPost, "/vertex-ai/v1/projects/test/locations/us-central1/publishers/google/models/veo-3.1-generate-preview:predictLongRunning", `{"instances":[{"prompt":"a landscape"}]}`, i18n.Translate("en", i18n.MsgAPIKeyContractRequired), http.StatusForbidden},
		{"DashScope error code", http.MethodPost, "/ali/api/v1/services/aigc/video-generation/video-synthesis", `{}`, `"code":"contract_required"`, http.StatusForbidden},
		{"Doubao error code", http.MethodPost, "/doubao/api/v3/images/generations", `{}`, `"code":"contract_required"`, http.StatusForbidden},
	} {
		t.Run(operation.name, func(t *testing.T) {
			request := httptest.NewRequest(operation.method, operation.path, strings.NewReader(operation.body))
			request.Header.Set("Authorization", "Bearer "+token.Key)
			if operation.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, operation.status, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), operation.response)
		})
	}
	var tasks int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
	assert.EqualValues(t, 1, tasks)
}

func TestAPIKeyContractPolicyHistoryIsScopedToApp(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	user := model.User{Username: "jimeng-history", Group: "default", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	var apps []model.Token
	for _, key := range []string{"jimengownerkey", "jimengotherkey"} {
		token := model.Token{UserId: user.Id, Key: key, Group: "retired", Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
		require.NoError(t, model.DB.Create(&token).Error)
		apps = append(apps, token)
	}
	tasks := []model.Task{
		{TaskID: "jimeng-history", UserId: user.Id, ClientProtocol: model.TaskClientProtocolJimeng, Status: model.TaskStatusQueued, PrivateData: model.TaskPrivateData{TokenId: apps[0].Id}},
		{TaskID: "kling-history", UserId: user.Id, ClientProtocol: model.TaskClientProtocolKlingV1, Status: model.TaskStatusQueued, AppID: apps[0].Id},
	}
	require.NoError(t, model.DB.Create(&tasks).Error)
	require.NoError(t, model.DB.Model(&tasks[0]).Update("app_id", nil).Error)
	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskRouter(engine)
	for _, operation := range []struct{ method, path, body string }{
		{http.MethodPost, "/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", `{"task_id":"jimeng-history"}`},
		{http.MethodGet, "/kling/v1/videos/text2video/kling-history", ""},
		{http.MethodGet, "/v1/tasks/jimeng-history", ""},
		{http.MethodGet, "/v1/tasks/kling-history/artifacts", ""},
	} {
		t.Run(operation.path, func(t *testing.T) {
			for index, app := range apps {
				request := httptest.NewRequest(operation.method, operation.path, strings.NewReader(operation.body))
				request.Header.Set("Authorization", "Bearer "+app.Key)
				if operation.body != "" {
					request.Header.Set("Content-Type", "application/json")
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Equal(t, []int{http.StatusOK, http.StatusNotFound}[index], response.Code, response.Body.String())
			}
		})
	}
}

func TestAPIKeyContractPolicyInvalidDisabledContractFailsClosed(t *testing.T) {
	setupRelayRouterTestDB(t)
	t.Cleanup(service.ResetContractEntityCacheForTest)
	user := model.User{Username: "invalid-contract-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 100, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	contract := model.CustomerContract{UserId: user.Id, Name: "invalid version", Enabled: false}
	require.NoError(t, model.DB.Create(&contract).Error)
	require.NoError(t, model.DB.Model(&contract).Update("version", 0).Error)
	key := model.Token{UserId: user.Id, Key: "invalidcontractversion", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ContractId: contract.Id}
	require.NoError(t, model.DB.Create(&key).Error)
	engine := gin.New()
	SetRelayRouter(engine)
	for _, setting := range []string{"true", "false"} {
		require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: setting}))
		request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		request.Header.Set("Authorization", "Bearer "+key.Key)
		result := httptest.NewRecorder()
		engine.ServeHTTP(result, request)
		assert.Equal(t, http.StatusServiceUnavailable, result.Code, result.Body.String())
	}
}

func TestAPIKeyContractPolicySelectsContractAndNativeDynamicPluginChannels(t *testing.T) {
	setupRelayRouterTestDB(t)
	t.Cleanup(service.ResetContractEntityCacheForTest)
	previousRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-route":1.5}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios)) })
	user := model.User{Username: "dynamic-contract-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 100, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	const publicModel = "veo-3.1-generate-preview"
	contractChannel := model.Channel{Name: "contract Vertex", Type: constant.ChannelTypeVertexAi, Key: "fixture-contract", Group: "contract-route", Models: publicModel, Status: common.ChannelStatusEnabled}
	nativeChannel := model.Channel{Name: "native Vertex", Type: constant.ChannelTypeVertexAi, Key: "fixture-native", Group: "default", Models: publicModel, Status: common.ChannelStatusEnabled}
	require.NoError(t, contractChannel.Insert())
	require.NoError(t, nativeChannel.Insert())
	contract, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: user.Id, Name: "Vertex contract", Enabled: true, Reason: "Verify plugin authorization",
		Rules: []model.CustomerContractEntityRuleInput{{PublicModel: publicModel, RouteGroup: contractChannel.Group, ChannelId: contractChannel.Id, RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	bound := model.Token{UserId: user.Id, Key: "vertexbound", Group: "retired", ContractId: contract.Id, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	unbound := model.Token{UserId: user.Id, Key: "vertexnative", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&[]model.Token{bound, unbound}).Error)

	engine := gin.New()
	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("vertex-ai")
	require.NoError(t, err)
	_, err = registry.RegisterFactory(source, jsplugin.Options{Key: "vertex-ai"})
	require.NoError(t, err)
	var selectedChannel int
	var selectedGroup string
	var usedContract bool
	handlers := func(generation *jsplugin.RoutingGeneration, group jsplugin.RouteGroup) []gin.HandlerFunc {
		chain := productionPluginRouteHandlers(generation, group)
		chain[len(chain)-1] = func(c *gin.Context) {
			selectedChannel = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
			selectedGroup = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
			usedContract = service.ActiveCustomerContract(c) != nil
			c.Status(http.StatusNoContent)
		}
		return chain
	}
	require.NoError(t, registry.SetGenerationPreparer(newPluginGenerationBuilder(engine.Routes(), nil, handlers).prepare))
	require.Empty(t, registry.RoutingErrors())
	engine.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)
	for _, test := range []struct {
		name, setting, key string
		channel            model.Channel
		contract           bool
	}{
		{"enabled bound key", "true", bound.Key, contractChannel, true},
		{"disabled native fallback", "false", unbound.Key, nativeChannel, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: test.setting}))
			request := httptest.NewRequest(http.MethodPost, "/vertex-ai/v1/projects/test/locations/us-central1/publishers/google/models/"+publicModel+":predictLongRunning", strings.NewReader(`{"instances":[{"prompt":"A landscape"}]}`))
			request.Header.Set("Authorization", "Bearer "+test.key)
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			engine.ServeHTTP(result, request)
			require.Equal(t, http.StatusNoContent, result.Code, result.Body.String())
			assert.Equal(t, test.channel.Id, selectedChannel)
			assert.Equal(t, test.channel.Group, selectedGroup)
			assert.Equal(t, test.contract, usedContract)
		})
	}
}

func TestAPIKeyContractPolicySelectsContractChannelsOnOfficialVideoRoutes(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	t.Cleanup(service.ResetContractEntityCacheForTest)
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	user := model.User{Username: "official-contract-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	tests := []struct {
		model, path, body string
		channelType       int
	}{
		{"kling-v2-master", "/kling/v1/videos/text2video", `{"model_name":"kling-v2-master","prompt":"A landscape","duration":"5"}`, constant.ChannelTypeKling},
		{"jimeng_v30", "/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", `{"req_key":"jimeng_v30","prompt":"A landscape","frames":121}`, constant.ChannelTypeJimeng},
		{"seedance-2-0", "/api/v3/contents/generations/tasks", `{"model":"seedance-2-0","content":[{"type":"text","text":"A landscape"}],"duration":4}`, constant.ChannelTypeSeedanceLink},
	}
	rules := make([]model.CustomerContractEntityRuleInput, 0, len(tests))
	for _, test := range tests {
		baseURL := "http://127.0.0.1:0"
		channel := model.Channel{Name: test.model, Type: test.channelType, Key: "local-test-channel", BaseURL: &baseURL, Group: "default", Models: test.model, Status: common.ChannelStatusEnabled}
		if test.channelType == constant.ChannelTypeSeedanceLink {
			channel.SetOtherSettings(relaykitdto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine})
			require.NoError(t, model.DB.Create(&channel).Error)
		} else {
			require.NoError(t, channel.Insert())
		}
		rules = append(rules, model.CustomerContractEntityRuleInput{PublicModel: test.model, RouteGroup: "default", ChannelId: channel.Id, RatioUnits: 80_000_000})
	}
	contract, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: user.Id, Name: "Official video contract", Enabled: true, Reason: "Verify official video channel admission", Rules: rules,
	})
	require.NoError(t, err)
	token := model.Token{UserId: user.Id, Key: "officialcontractbound", Group: "retired", ContractId: contract.Id, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	engine := gin.New()
	var channelID, contractID int
	engine.Use(func(c *gin.Context) {
		c.Next()
		channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		contractID = 0
		if active := service.ActiveCustomerContract(c); active != nil {
			contractID = active.Id
		}
	})
	SetVideoRouter(engine)
	for index, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer "+token.Key)
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			engine.ServeHTTP(result, request)
			assert.Equal(t, contract.Id, contractID, result.Body.String())
			assert.Equal(t, rules[index].ChannelId, channelID, result.Body.String())
		})
	}
}

func TestAPIKeyContractPolicyDiscoveryUsesPositiveContractAndNativeFallback(t *testing.T) {
	setupRelayRouterTestDB(t)
	previousRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"contract-visible":1,"native-visible":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatio)) })
	t.Cleanup(service.ResetContractEntityCacheForTest)
	user := model.User{Username: "contract-model-user", Group: "default", Status: common.UserStatusEnabled, Quota: 100, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	channel := model.Channel{Name: "contract-policy-channel", Group: "default", Models: "contract-visible,native-visible", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&channel).Error)
	for _, name := range []string{"contract-visible", "native-visible"} {
		require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: name, ChannelId: channel.Id, Enabled: true}).Error)
	}
	contract, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{
		UserId: user.Id, AdminUserId: user.Id, Name: "Policy test contract", Enabled: true, Reason: "Enable test model",
		Rules: []model.CustomerContractEntityRuleInput{{PublicModel: "contract-visible", RouteGroup: "default", ChannelId: channel.Id, RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	bound := model.Token{UserId: user.Id, Key: "contractpolicybound", Group: "retired-group", ContractId: contract.Id, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	unbound := model.Token{UserId: user.Id, Key: "contractpolicyunbound", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&bound).Error)
	require.NoError(t, model.DB.Create(&unbound).Error)
	engine := gin.New()
	SetRelayRouter(engine)
	list := func(key string) string {
		request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		request.Header.Set("Authorization", "Bearer "+key)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		return response.Body.String()
	}
	for _, enabled := range []string{"true", "false"} {
		require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: enabled}))
		body := list(bound.Key)
		assert.Contains(t, body, `"id":"contract-visible"`)
		assert.NotContains(t, body, `"id":"native-visible"`)
	}
	assert.Contains(t, list(unbound.Key), `"id":"native-visible"`)
	require.NoError(t, model.DB.Model(&model.CustomerContract{}).Where("id = ?", contract.Id).Update("enabled", false).Error)
	service.ResetContractEntityCacheForTest()
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{model.RequireAPIKeyContractOption: "true"}))
	assert.JSONEq(t, `{"success":true,"data":[],"object":"list"}`, list(bound.Key))
}
