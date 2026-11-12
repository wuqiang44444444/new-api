package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDefaultAffinityTest(t *testing.T, redisClient *redis.Client) {
	t.Helper()
	previousCache, previousRedis := defaultAffinityCache, common.RedisEnabled
	setting := operation_setting.GetChannelAffinitySetting()
	previousSetting := *setting
	cache := newDefaultAffinityCache(redisClient)
	defaultAffinityCache = func() *cachex.HybridCache[int] { return cache }
	common.RedisEnabled = redisClient != nil
	setting.Enabled, setting.SwitchOnSuccess = true, true
	t.Cleanup(func() {
		defaultAffinityCache, common.RedisEnabled = previousCache, previousRedis
		*setting = previousSetting
	})
}

func newDefaultAffinityTestRequest(t *testing.T, method, path, modelName, group, body string) *RetryParam {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, 19)
	common.SetContextKey(c, constant.ContextKeyTokenId, 101)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	return &RetryParam{Ctx: c, TokenGroup: group, ModelName: modelName, RequestPath: path}
}

func TestDefaultChannelAffinityScope(t *testing.T) {
	setupDefaultAffinityTest(t, nil)
	for _, tc := range []struct {
		name, method, path, body string
		eligible                 bool
	}{
		{"chat", "POST", "/v1/chat/completions", `{"messages":[{"role":"user","content":"hello"}]}`, true},
		{"messages", "POST", "/v1/messages", `{"system":[{"type":"text","text":"context","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hello"}]}`, true},
		{"responses", "POST", "/v1/responses", `{"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`, true},
		{"paired function calls", "POST", "/v1/responses", `{"input":[{"type":"function_call","call_id":"a","name":"weather","arguments":"{}"},{"type":"function_call","call_id":"b","name":"weather","arguments":"{}"},{"type":"function_call_output","call_id":"b","output":[{"type":"input_text","text":"15 degrees"}]},{"type":"function_call_output","call_id":"a","output":"20 degrees"}]}`, true},
		{"unpaired function result", "POST", "/v1/responses", `{"input":[{"type":"function_call_output","call_id":"old","output":"text"}]}`, false},
		{"stored function result", "POST", "/v1/responses", `{"input":[{"type":"function_call","call_id":"a","name":"weather","arguments":"{}"},{"type":"function_call_output","call_id":"a","id":"stored_item","output":"text"}]}`, false},
		{"media function result", "POST", "/v1/responses", `{"input":[{"type":"function_call","call_id":"a","name":"weather","arguments":"{}"},{"type":"function_call_output","call_id":"a","output":[{"type":"input_file","file_id":"old"}]}]}`, false},
		{"websocket", "GET", "/v1/responses", `{"input":"hello"}`, false},
		{"compact", "POST", "/v1/responses/compact", `{"input":"hello"}`, false},
		{"completions", "POST", "/v1/completions", `{"prompt":"hello"}`, false},
		{"playground", "POST", "/pg/chat/completions", `{"messages":[{"role":"user","content":"hello"}]}`, false},
		{"prior response", "POST", "/v1/responses", `{"input":"hello","Previous_Response_Id":"resp_old"}`, false},
		{"conversation", "POST", "/v1/responses", `{"input":"hello","conversation":{"id":"conv_old"}}`, false},
		{"stored prompt", "POST", "/v1/responses", `{"input":"hello","prompt":{"id":"pmpt_old"}}`, false},
		{"background", "POST", "/v1/responses", `{"input":"hello","background":true}`, false},
		{"response file", "POST", "/v1/responses", `{"input":[{"role":"user","content":[{"type":"input_file","file_id":"file_old"}]}]}`, false},
		{"vector store", "POST", "/v1/responses", `{"input":"hello","tools":[{"type":"file_search","vector_store_ids":["vs_old"]}]}`, false},
		{"tool container", "POST", "/v1/responses", `{"input":"hello","tools":[{"type":"code_interpreter","container":"cntr_old"}]}`, false},
		{"claude container", "POST", "/v1/messages", `{"messages":[{"role":"user","content":"hello"}],"container":"old"}`, false},
		{"image in chat", "POST", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test"}}]}]}`, false},
		{"unknown input item", "POST", "/v1/responses", `{"input":[{"type":"item_reference","id":"old"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			param := newDefaultAffinityTestRequest(t, tc.method, tc.path, "test-model", "default", tc.body)
			prepareDefaultChannelAffinity(param)
			_, eligible := param.Ctx.Get(defaultAffinityContextKey)
			assert.Equal(t, tc.eligible, eligible)
		})
	}
	for _, reason := range []string{"plugin", "disabled"} {
		t.Run(reason, func(t *testing.T) {
			param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/responses", "test-model", "default", `{"input":"hello"}`)
			if reason == "plugin" {
				param.Ctx.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{})
			} else {
				operation_setting.GetChannelAffinitySetting().Enabled = false
			}
			prepareDefaultChannelAffinity(param)
			_, eligible := param.Ctx.Get(defaultAffinityContextKey)
			assert.False(t, eligible)
			if reason == "disabled" {
				assert.Empty(t, RequestPolicy(param.Ctx).Events())
			}
		})
	}
}

func TestDefaultChannelAffinitySeparatesAPIKeys(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var client *redis.Client
			if backend == "redis" {
				server := miniredis.RunT(t)
				client = redis.NewClient(&redis.Options{Addr: server.Addr()})
				t.Cleanup(func() { require.NoError(t, client.Close()) })
			}
			db := setupChannelSelectAutoGroupsTest(t)
			setupDefaultAffinityTest(t, client)
			const modelName = "key-affinity-test"
			createChannelSelectAutoGroupsChannel(t, db, 901, "default", modelName)
			createChannelSelectAutoGroupsChannel(t, db, 902, "default", modelName)
			model.InitChannelCache()

			selectForKey := func(tokenID int) (*RetryParam, int) {
				t.Helper()
				request := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/responses", modelName, "default", `{"input":"hello"}`)
				common.SetContextKey(request.Ctx, constant.ContextKeyTokenId, tokenID)
				channel, _, selectErr := SelectChannelForRequest(request.Ctx, modelName, request)
				require.Nil(t, selectErr)
				require.NotNil(t, channel)
				return request, channel.Id
			}
			first, firstChannel := selectForKey(101)
			require.Equal(t, "binding_missing", RequestPolicy(first.Ctx).Events()[0].Decision.Reason)
			RecordDefaultChannelAffinitySuccess(first.Ctx, firstChannel, &relaycommon.RelayInfo{})

			second, secondChannel := selectForKey(102)
			require.Equal(t, "binding_missing", RequestPolicy(second.Ctx).Events()[0].Decision.Reason)
			RecordDefaultChannelAffinitySuccess(second.Ctx, secondChannel, &relaycommon.RelayInfo{})

			for _, key := range []struct{ id, channel int }{{101, firstChannel}, {102, secondChannel}} {
				request, selected := selectForKey(key.id)
				assert.Equal(t, key.channel, selected)
				require.Equal(t, "binding_used", RequestPolicy(request.Ctx).Events()[0].Decision.Reason)
			}

			withoutKey, selected := selectForKey(0)
			_, hasAffinity := withoutKey.Ctx.Get(defaultAffinityContextKey)
			assert.False(t, hasAffinity)
			RecordDefaultChannelAffinitySuccess(withoutKey.Ctx, selected, &relaycommon.RelayInfo{})
			keys, err := defaultAffinityCache().Keys()
			require.NoError(t, err)
			assert.Len(t, keys, 2)
		})
	}
}

func TestDefaultChannelAffinityKeepsInitialLayerAndScope(t *testing.T) {
	for _, inMemory := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory=%t", inMemory), func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			setupDefaultAffinityTest(t, nil)
			const modelName = "affinity-test"
			for _, id := range []int{301, 302, 303} {
				createChannelSelectAutoGroupsChannel(t, db, id, "default", modelName)
			}
			createChannelSelectAutoGroupsChannel(t, db, 304, "vip", modelName)
			for _, table := range []any{&model.Channel{}, &model.Ability{}} {
				idColumn := "id"
				if _, ok := table.(*model.Ability); ok {
					idColumn = "channel_id"
				}
				require.NoError(t, db.Model(table).Where(idColumn+" IN ?", []int{301, 302, 304}).Update("priority", 2).Error)
			}
			common.MemoryCacheEnabled = inMemory
			model.InitChannelCache()
			makeRequest := func(group string) *RetryParam {
				return newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/chat/completions", modelName, group, `{"messages":[{"role":"user","content":"hello"}]}`)
			}
			seed := makeRequest("default")
			first, _, selectErr := SelectChannelForRequest(seed.Ctx, modelName, seed)
			require.Nil(t, selectErr)
			require.NotNil(t, first)
			ctx, cancel := context.WithCancel(seed.Ctx.Request.Context())
			seed.Ctx.Request = seed.Ctx.Request.WithContext(ctx)
			cancel() // A complete response can be followed by a client disconnect.
			RecordDefaultChannelAffinitySuccess(seed.Ctx, first.Id, &relaycommon.RelayInfo{})
			state := seed.Ctx.MustGet(defaultAffinityContextKey).(*defaultAffinityRequest)
			binding, found, err := defaultAffinityCache().Get(state.key)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, first.Id, binding)

			hotRequest := makeRequest("default")
			same, _, selectErr := SelectChannelForRequest(hotRequest.Ctx, modelName, hotRequest)
			require.Nil(t, selectErr)
			require.NotNil(t, same)
			assert.Equal(t, first.Id, same.Id)
			assert.Equal(t, "binding_used", RequestPolicy(hotRequest.Ctx).Events()[0].Decision.Reason)
			hotRequest.SetRetry(1)
			lower, _, err := CacheGetRandomSatisfiedChannel(hotRequest)
			require.NoError(t, err)
			require.NotNil(t, lower)
			require.Equal(t, 303, lower.Id)
			RecordDefaultChannelAffinitySuccess(hotRequest.Ctx, lower.Id, &relaycommon.RelayInfo{})
			binding, found, err = defaultAffinityCache().Get(state.key)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, first.Id, binding, "success in a lower layer must not replace the initial binding")

			require.NoError(t, defaultAffinityCache().SetWithTTL(state.key, 303, defaultAffinityTTL))
			stale := makeRequest("default")
			selected, _, selectErr := SelectChannelForRequest(stale.Ctx, modelName, stale)
			require.Nil(t, selectErr)
			require.NotNil(t, selected)
			assert.Contains(t, []int{301, 302}, selected.Id)

			auto := makeRequest("auto")
			common.SetContextKey(auto.Ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
			common.SetContextKey(auto.Ctx, constant.ContextKeyTokenCrossGroupRetry, true)
			vip, group, selectErr := SelectChannelForRequest(auto.Ctx, modelName, auto)
			require.Nil(t, selectErr)
			require.NotNil(t, vip)
			assert.Equal(t, "vip", group)
			assert.Equal(t, 304, vip.Id)
			fallback, group, err := CacheGetRandomSatisfiedChannel(auto)
			require.NoError(t, err)
			require.NotNil(t, fallback)
			assert.Equal(t, "default", group)
			RecordDefaultChannelAffinitySuccess(auto.Ctx, fallback.Id, &relaycommon.RelayInfo{})
			autoState := auto.Ctx.MustGet(defaultAffinityContextKey).(*defaultAffinityRequest)
			_, found, err = defaultAffinityCache().Get(autoState.key)
			require.NoError(t, err)
			assert.False(t, found, "success in another Auto group must not bind the first group")

			contract := makeRequest("default")
			common.SetContextKey(contract.Ctx, constant.ContextKeyTokenCrossGroupRetry, true)
			contract.Ctx.Set(contractRequestKey, &model.ContractEntitySnapshot{Id: 1, Version: 1, Enabled: true, Rules: []model.ContractEntityRule{
				{PublicModel: modelName, ChannelId: 304, RouteGroup: "vip", RatioUnits: 100_000_000},
				{PublicModel: modelName, ChannelId: 301, RouteGroup: "default", RatioUnits: 100_000_000},
			}})
			contractChannel, group, selectErr := SelectChannelForRequest(contract.Ctx, modelName, contract)
			require.Nil(t, selectErr)
			require.NotNil(t, contractChannel)
			contractGroups := map[int]string{304: "vip", 301: "default"}
			require.Contains(t, contractGroups, contractChannel.Id)
			assert.Equal(t, contractGroups[contractChannel.Id], group)
			assert.Zero(t, contract.Ctx.MustGet(defaultAffinityContextKey).(*defaultAffinityRequest).preferred, "the ordinary binding is not visible inside a contract")

			// The contract remains the same. A channel in its other route
			// group becomes the only eligible candidate before the retry.
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", contractChannel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
			require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", contractChannel.Id).Update("enabled", false).Error)
			model.InitChannelCache()
			contract.SetRetry(1)
			retryChannel, retryGroup, err := CacheGetRandomSatisfiedChannel(contract)
			require.NoError(t, err)
			require.NotNil(t, retryChannel)
			require.Contains(t, contractGroups, retryChannel.Id)
			assert.NotEqual(t, contractChannel.Id, retryChannel.Id)
			assert.Equal(t, contractGroups[retryChannel.Id], retryGroup)
			assert.NotEqual(t, group, retryGroup)
			assert.Equal(t, retryGroup, common.GetContextKeyString(contract.Ctx, constant.ContextKeyUsingGroup))
			RecordDefaultChannelAffinitySuccess(contract.Ctx, retryChannel.Id, &relaycommon.RelayInfo{})
			contractState := contract.Ctx.MustGet(defaultAffinityContextKey).(*defaultAffinityRequest)
			binding, found, err = defaultAffinityCache().Get(contractState.key)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, retryChannel.Id, binding)
		})
	}
}

func TestDefaultChannelAffinitySkipsExistingRuleCacheMiss(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupDefaultAffinityTest(t, nil)
	setting := operation_setting.GetChannelAffinitySetting()
	setting.Rules = []operation_setting.ChannelAffinityRule{
		{Name: "codex-test", ModelRegex: []string{"^gpt-"}, PathRegex: []string{"^/v1/responses$"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "gjson", Path: "prompt_cache_key"}}, IncludeRuleName: true},
		{Name: "claude-test", ModelRegex: []string{"^claude-"}, PathRegex: []string{"^/v1/messages$"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "gjson", Path: "metadata.user_id"}}, IncludeRuleName: true},
	}
	createChannelSelectAutoGroupsChannel(t, db, 402, "default", "gpt-affinity")
	createChannelSelectAutoGroupsChannel(t, db, 403, "default", "claude-affinity")
	model.InitChannelCache()
	for _, tc := range []struct{ path, modelName, body, plain string }{
		{"/v1/responses", "gpt-affinity", `{"input":"hello","prompt_cache_key":"affinity-legacy-test"}`, `{"input":"hello"}`},
		{"/v1/messages", "claude-affinity", `{"messages":[{"role":"user","content":"hello"}],"metadata":{"user_id":"affinity-legacy-test"}}`, `{"messages":[{"role":"user","content":"hello"}]}`},
	} {
		seed := newDefaultAffinityTestRequest(t, http.MethodPost, tc.path, tc.modelName, "default", tc.plain)
		seeded, _, selectErr := SelectChannelForRequest(seed.Ctx, tc.modelName, seed)
		require.Nil(t, selectErr)
		RecordDefaultChannelAffinitySuccess(seed.Ctx, seeded.Id, &relaycommon.RelayInfo{})

		explicit := newDefaultAffinityTestRequest(t, http.MethodPost, tc.path, tc.modelName, "default", tc.body)
		_, found := GetPreferredChannelByAffinity(explicit.Ctx, tc.modelName, "default")
		require.False(t, found)
		selected, _, selectErr := SelectChannelForRequest(explicit.Ctx, tc.modelName, explicit)
		require.Nil(t, selectErr)
		_, defaultUsed := explicit.Ctx.Get(defaultAffinityContextKey)
		assert.False(t, defaultUsed)
		RecordChannelAffinity(explicit.Ctx, selected.Id)
		t.Cleanup(func() { ClearCurrentChannelAffinityCache(explicit.Ctx) })
		bound, found := GetPreferredChannelByAffinity(explicit.Ctx, tc.modelName, "default")
		require.True(t, found)
		assert.Equal(t, selected.Id, bound)
	}
}

func TestDefaultChannelAffinityRespectsSwitchOnSuccess(t *testing.T) {
	for _, switchOnSuccess := range []bool{false, true} {
		t.Run(fmt.Sprintf("switch=%t", switchOnSuccess), func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			setupDefaultAffinityTest(t, nil)
			operation_setting.GetChannelAffinitySetting().SwitchOnSuccess = switchOnSuccess
			const modelName = "same-tier-retry"
			createChannelSelectAutoGroupsChannel(t, db, 601, "default", modelName)
			createChannelSelectAutoGroupsChannel(t, db, 602, "default", modelName)
			model.InitChannelCache()
			makeRequest := func() *RetryParam {
				return newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/responses", modelName, "default", `{"input":"hello"}`)
			}
			seed := makeRequest()
			first, _, selectErr := SelectChannelForRequest(seed.Ctx, modelName, seed)
			require.Nil(t, selectErr)
			require.NotNil(t, first)
			RecordDefaultChannelAffinitySuccess(seed.Ctx, first.Id, &relaycommon.RelayInfo{})

			request := makeRequest()
			initial, _, selectErr := SelectChannelForRequest(request.Ctx, modelName, request)
			require.Nil(t, selectErr)
			require.Equal(t, first.Id, initial.Id)
			model.CacheUpdateChannelStatus(first.Id, common.ChannelStatusManuallyDisabled)
			request.SetRetry(1)
			fallback, _, err := CacheGetRandomSatisfiedChannel(request)
			require.NoError(t, err)
			require.NotNil(t, fallback)
			require.NotEqual(t, first.Id, fallback.Id)
			RecordDefaultChannelAffinitySuccess(request.Ctx, fallback.Id, &relaycommon.RelayInfo{})
			state := request.Ctx.MustGet(defaultAffinityContextKey).(*defaultAffinityRequest)
			bound, found, err := defaultAffinityCache().Get(state.key)
			require.NoError(t, err)
			require.True(t, found)
			if switchOnSuccess {
				assert.Equal(t, fallback.Id, bound)
			} else {
				assert.Equal(t, first.Id, bound)
			}
		})
	}
}

func TestDefaultChannelAffinityRedisFailureFallsThrough(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	setupDefaultAffinityTest(t, client)
	const modelName = "redis-affinity"
	createChannelSelectAutoGroupsChannel(t, db, 501, "default", modelName)
	model.InitChannelCache()
	makeRequest := func() *RetryParam {
		return newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/responses", modelName, "default", `{"input":"hello"}`)
	}
	redisServer.SetError("ERR cache unavailable")
	failedRead := makeRequest()
	selected, _, selectErr := SelectChannelForRequest(failedRead.Ctx, modelName, failedRead)
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	assert.Equal(t, 501, selected.Id)
	assert.Equal(t, "cache_unavailable", RequestPolicy(failedRead.Ctx).Events()[0].Decision.Reason)
	redisServer.SetError("")
	RecordDefaultChannelAffinitySuccess(failedRead.Ctx, selected.Id, &relaycommon.RelayInfo{})
	assert.Empty(t, redisServer.Keys(), "a request whose read failed does not replace an unread binding")

	failedWrite := makeRequest()
	selected, _, selectErr = SelectChannelForRequest(failedWrite.Ctx, modelName, failedWrite)
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	redisServer.SetError("ERR cache unavailable")
	RecordDefaultChannelAffinitySuccess(failedWrite.Ctx, selected.Id, &relaycommon.RelayInfo{})
	assert.Equal(t, http.StatusOK, failedWrite.Ctx.Writer.Status())
	redisServer.SetError("")
	assert.Empty(t, redisServer.Keys())

	recovered := makeRequest()
	selected, _, selectErr = SelectChannelForRequest(recovered.Ctx, modelName, recovered)
	require.Nil(t, selectErr)
	require.NotNil(t, selected)
	RecordDefaultChannelAffinitySuccess(recovered.Ctx, selected.Id, &relaycommon.RelayInfo{})
	state := recovered.Ctx.MustGet(defaultAffinityContextKey).(*defaultAffinityRequest)
	require.Equal(t, []string{defaultAffinityCache().FullKey(state.key)}, redisServer.Keys())
	assert.NotEqual(t, cachex.Namespace(channelAffinityCacheNamespace).FullKey(state.key), redisServer.Keys()[0])
}

func TestDefaultChannelAffinityCapacityDoesNotEvictLegacyBinding(t *testing.T) {
	setupDefaultAffinityTest(t, nil)
	const oldRuleKey = "affinity-capacity-test:legacy"
	legacy := getChannelAffinityCache()
	require.NoError(t, legacy.SetWithTTL(oldRuleKey, 701, defaultAffinityTTL))
	t.Cleanup(func() { _, _ = legacy.DeleteMany([]string{oldRuleKey}) })
	defaults := defaultAffinityCache()
	capacity, _ := defaults.Capacity()
	require.NoError(t, defaults.SetWithTTL("first", 702, defaultAffinityTTL))
	for i := range capacity {
		require.NoError(t, defaults.SetWithTTL(fmt.Sprintf("new-%d", i), 703, defaultAffinityTTL))
	}
	_, found, err := defaults.Get("first")
	require.NoError(t, err)
	assert.False(t, found)
	legacyChannel, found, err := legacy.Get(oldRuleKey)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 701, legacyChannel)
}
