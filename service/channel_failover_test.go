package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelFailoverEvidenceAndRecovery(t *testing.T) {
	old := operation_setting.AutomaticRetryStatusCodesToString()
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("502,503,529"))
	t.Cleanup(func() { require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(old)) })
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	scope := ChannelFailoverScope{ChannelID: 781, Revision: "test-v1", Model: "text", Operation: "chat", Slot: -1}
	for _, status := range []int{502, 502, 429, 502, 503} {
		state, err := observeChannelFailoverHealth(scope, status)
		require.NoError(t, err)
		assert.Empty(t, state.State)
	}
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("503,529"))
	state, err := observeChannelFailoverHealth(scope, 529)
	require.NoError(t, err)
	assert.Len(t, state.Samples, 2, "the no-longer-retryable status breaks the streak")
	state, err = observeChannelFailoverHealth(scope, 0)
	require.NoError(t, err)
	assert.Equal(t, "cooldown", state.State)
	assert.True(t, state.NextCheck.After(time.Now()))
	otherScope := scope
	otherScope.Revision = "test-v2"
	assert.Empty(t, readChannelFailoverHealth(t.Context(), []ChannelFailoverScope{otherScope})[otherScope].State)
	otherScope = scope
	otherScope.Model = "other"
	assert.Empty(t, readChannelFailoverHealth(t.Context(), []ChannelFailoverScope{otherScope})[otherScope].State)

	// Failed and successful checks alter only the temporary routing evidence.
	for _, check := range []struct {
		name, state, result, expected string
		trial                         bool
	}{
		{"executor error", "cooldown", "unconfirmed", "pending", false},
		{"healthy probe", "pending", "success", "trial_ready", false},
		{"failed trial", "trial_ready", "failed", "cooldown", true},
		{"interrupted trial", "trial_ready", "unconfirmed", "trial_ready", true},
		{"successful trial", "trial_ready", "success", "", true},
	} {
		t.Run(check.name, func(t *testing.T) {
			_, _, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, now time.Time) bool {
				next.State, next.Lease, next.LeaseUntil = check.state, "valid", now.Add(time.Minute)
				return true
			})
			require.NoError(t, err)
			committed, err := CompleteChannelFailoverCheck(scope, "stale", "success", 200, check.trial)
			require.NoError(t, err)
			assert.False(t, committed, "a stale lease cannot report a recorded result")
			assert.Equal(t, check.state, readChannelFailoverHealth(t.Context(), []ChannelFailoverScope{scope})[scope].State)
			committed, err = CompleteChannelFailoverCheck(scope, "valid", check.result, 200, check.trial)
			require.NoError(t, err)
			assert.True(t, committed)
			assert.Equal(t, check.expected, readChannelFailoverHealth(t.Context(), []ChannelFailoverScope{scope})[scope].State)
		})
	}
}

func TestChannelAffinityCompareAndSwapAcrossNodes(t *testing.T) {
	redisServer := miniredis.RunT(t)
	for _, shared := range []bool{false, true} {
		var client *redis.Client
		if shared {
			client = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
		}
		create := func() *cachex.HybridCache[int] {
			return cachex.NewHybridCache(cachex.HybridCacheConfig[int]{
				Namespace: "failover-affinity-test", Redis: client, RedisCodec: cachex.IntCodec{},
				Memory: func() *hot.HotCache[string, int] { return hot.NewHotCache[string, int](hot.LRU, 8).Build() },
			})
		}
		first := create()
		second := first
		if shared {
			second = create()
		}
		require.NoError(t, first.SetWithTTL("session", 20, time.Minute))
		release := make(chan struct{})
		results := make(chan bool, 2)
		for _, attempt := range []struct {
			cache   *cachex.HybridCache[int]
			channel int
		}{{first, 21}, {second, 22}} {
			go func() {
				<-release
				ok, err := attempt.cache.CompareAndSwapWithTTL("session", common.GetPointer(20), attempt.channel, time.Minute)
				results <- ok && err == nil
			}()
		}
		close(release)
		assert.NotEqual(t, <-results, <-results, "only one replacement of the observed binding can win")
		ok, err := first.CompareAndSwapWithTTL("session", common.GetPointer(20), 20, time.Minute)
		require.NoError(t, err)
		assert.False(t, ok, "a late success cannot restore the previous binding")
	}
}

func TestChannelFailoverInputBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, kind string
		owner                  bool
	}{
		{"plain Claude", "/v1/messages", `{"messages":[{"role":"user","content":"hi"}]}`, failoverText, false},
		{"Claude thinking history", "/v1/messages", `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"private","signature":"opaque"}]}]}`, "", false},
		{"image", "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/image.png"}}]}]}`, "", false},
		{"Claude paired tool history", "/v1/messages", `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"demo","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":"done"}]}]}`, failoverTools, false},
		{"Claude unpaired tool history", "/v1/messages", `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":"done"}]}]}`, "", false},
		{"function tools", "/v1/responses", `{"input":[{"type":"function_call","call_id":"c","name":"demo","arguments":"{}"},{"type":"function_call_output","call_id":"c","output":"done"}]}`, failoverTools, false},
		{"continuation", "/v1/responses", `{"input":"hi","Previous_Response_Id":"resp_old"}`, "", true},
		{"stored file", "/v1/responses", `{"input":[{"role":"user","content":[{"type":"input_file","file_id":"file_old"}]}]}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			param := newDefaultAffinityTestRequest(t, http.MethodPost, tc.path, "shared-model", "default", tc.body)
			content := inspectFailoverContent(param.Ctx, ChannelFailoverOperation(tc.path))
			assert.Equal(t, tc.kind, content.kind)
			assert.Equal(t, tc.owner, len(content.ownerReferences) > 0)
		})
	}
	source := &model.Channel{Type: constant.ChannelTypeAnthropic}
	target := &model.Channel{Type: constant.ChannelTypeOpenAI}
	assert.True(t, canMigrateChannelContent("messages", failoverText, source, target))
	assert.False(t, canMigrateChannelContent("messages", failoverTools, source, target))
	assert.False(t, canMigrateChannelContent("messages", "", source, target))
	multiKey := &model.Channel{Type: constant.ChannelTypeOpenAI}
	multiKey.ChannelInfo.IsMultiKey = true
	assert.False(t, canMigrateChannelContent("messages", failoverText, source, multiKey), "a multi-key backup is outside the verified scope")
	global := model_setting.GetGlobalSettings()
	previousPassThrough := global.PassThroughRequestEnabled
	global.PassThroughRequestEnabled = true
	assert.False(t, canMigrateChannelContent("messages", failoverText, source, target), "global pass-through bypasses the adapters")
	global.PassThroughRequestEnabled = previousPassThrough

	param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/responses", "shared-model", "default", `{"input":"hi"}`)
	common.SetContextKey(param.Ctx, constant.ContextKeyTokenId, 17)
	key := responseOwnerKey(param, "response:opaque")
	assert.NotContains(t, key, "opaque")
	common.SetContextKey(param.Ctx, constant.ContextKeyTokenId, 18)
	assert.NotEqual(t, key, responseOwnerKey(param, "response:opaque"))
	assert.False(t, strings.Contains(key, "shared-model"))
}

func TestChannelFailoverHealthReadsRedisManualBypassDuringOutage(t *testing.T) {
	oldRedis, oldEnabled, oldHealth := common.RDB, common.RedisEnabled, sharedFailoverHealth
	server := miniredis.RunT(t)
	common.RDB, common.RedisEnabled = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1}), true
	client := common.RDB
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		common.RDB, common.RedisEnabled, sharedFailoverHealth = oldRedis, oldEnabled, oldHealth
	})
	sharedFailoverHealth = func() *cachex.HybridCache[ChannelFailoverHealth] {
		return cachex.NewHybridCache(cachex.HybridCacheConfig[ChannelFailoverHealth]{
			Namespace: failoverNamespace, Redis: client, RedisCodec: cachex.JSONCodec[ChannelFailoverHealth]{}, RedisOpTimeout: failoverRedisTimeout,
		})
	}
	scope := ChannelFailoverScope{ChannelID: 982, Revision: "v1", Model: "test", Operation: "chat", Slot: -1}
	require.NoError(t, SetChannelFailoverBypass(scope, time.Minute, "test bypass"))
	assert.True(t, readChannelFailoverHealth(context.Background(), []ChannelFailoverScope{scope})[scope].bypassed(time.Now()))
	server.Close()
	localFailoverReads.Delete(scope.key())
	assert.True(t, readChannelFailoverHealth(context.Background(), []ChannelFailoverScope{scope})[scope].bypassed(time.Now()))
}

func TestLayeredChannelAffinityUsesAuthorizedTierAndClearsIndependently(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupDefaultAffinityTest(t, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
		"channel_affinity_setting.enabled":          "true",
		"channel_affinity_setting.layered_affinity": "true",
	}))
	const modelName = "layered-affinity-regression"
	for _, id := range []int{991, 992} {
		createChannelSelectAutoGroupsChannel(t, db, id, "default", modelName)
	}
	createChannelSelectAutoGroupsChannel(t, db, 993, "other", modelName)
	model.InitChannelCache()
	selectFor := func(user, token int) (*RetryParam, int) {
		request := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/chat/completions", modelName, "default", `{"messages":[{"role":"user","content":"hello"}]}`)
		common.SetContextKey(request.Ctx, constant.ContextKeyUserId, user)
		common.SetContextKey(request.Ctx, constant.ContextKeyTokenId, token)
		channel, _, selectErr := SelectChannelForRequest(request.Ctx, modelName, request)
		require.Nil(t, selectErr)
		require.NotNil(t, channel)
		return request, channel.Id
	}
	first, id := selectFor(19, 101)
	firstState := layeredAffinityFrom(first.Ctx)
	require.True(t, RecordLayeredAffinityAccepted(first.Ctx, id))
	other := 991
	if id == other {
		other = 992
	}
	require.NoError(t, defaultAffinityCache().SetWithTTL(firstState.token.key, other, time.Minute))
	_, chosen := selectFor(19, 101)
	assert.Equal(t, other, chosen, "the key binding takes precedence over the user binding")
	second, chosen := selectFor(19, 102)
	assert.Equal(t, id, chosen, "another key can borrow the user's binding")
	require.True(t, RecordLayeredAffinityAccepted(second.Ctx, chosen))
	unrelated, chosen := selectFor(20, 103)
	assert.NotEqual(t, firstState.user.key, layeredAffinityFrom(unrelated.Ctx).user.key)
	assert.Contains(t, []int{991, 992}, chosen)

	require.NoError(t, userAffinityCache().SetWithTTL(firstState.user.key, 993, time.Minute))
	_, chosen = selectFor(19, 104)
	assert.Contains(t, []int{991, 992}, chosen, "a stale user binding cannot cross authorized groups")

	deleted, err := ClearChannelAffinityLayer("api_key")
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)
	_, found, err := userAffinityCache().Get(firstState.user.key)
	require.NoError(t, err)
	assert.True(t, found)
}

func TestChannelFailoverContinuationKeepsAuthorizedOrigin(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupDefaultAffinityTest(t, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
		"channel_affinity_setting.failover_mode":        "enabled",
		"channel_affinity_setting.failover_channel_ids": "[994]",
	}))
	const modelName = "responses-owner-regression"
	for _, id := range []int{994, 995} {
		createChannelSelectAutoGroupsChannel(t, db, id, "default", modelName)
	}
	model.InitChannelCache()
	param := func(token int) *RetryParam {
		request := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/responses", modelName, "default", `{"input":"hello","previous_response_id":"resp_owned"}`)
		common.SetContextKey(request.Ctx, constant.ContextKeyTokenId, token)
		return request
	}
	ownerChannel, err := model.CacheGetChannel(994)
	require.NoError(t, err)
	scope := channelFailoverScope(ownerChannel, modelName, "responses", -1)
	require.NoError(t, SetChannelFailoverBypass(scope, time.Minute, "fixture outage"))
	key := responseOwnerKey(param(19), "response:resp_owned")
	require.NoError(t, responseChannelOwners().SetWithTTL(key, responseChannelOwner{ChannelID: 994, Revision: scope.Revision}, time.Minute))
	request := param(19)
	channel, _, selectErr := SelectChannelForRequest(request.Ctx, modelName, request)
	require.Nil(t, selectErr)
	require.NotNil(t, channel)
	assert.Equal(t, 994, channel.Id, "continuation intentionally keeps its own channel during a temporary bypass")

	// Another token cannot read the owner. Its request keeps native routing and
	// stays outside failover, so nothing bypasses or supplements it.
	request = param(20)
	channel, _, selectErr = SelectChannelForRequest(request.Ctx, modelName, request)
	require.Nil(t, selectErr)
	require.NotNil(t, channel)
	assert.Empty(t, failoverRequestFrom(request.Ctx).content.kind)
	assert.Zero(t, failoverRequestFrom(request.Ctx).owner.ChannelID)
}

// An unverified session on a rollout channel is outside failover; once its
// binding loses eligibility it keeps the existing reselection behavior.
func TestUnverifiedRolloutSessionReselectsWhenBindingLosesEligibility(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupDefaultAffinityTest(t, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	rules := `[{"name":"unverified session","model_regex":[".*"],"key_sources":[{"type":"request_header","key":"X-Session"}],"session_mode":"prefer","include_rule_name":true},{"name":"codex cli trace"},{"name":"claude cli trace"}]`
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
		"channel_affinity_setting.enabled":              "true",
		"channel_affinity_setting.rules":                rules,
		"channel_affinity_setting.failover_mode":        "enabled",
		"channel_affinity_setting.failover_channel_ids": "[996]",
	}))
	const modelName = "unverified-rollout-regression"
	for _, id := range []int{996, 997} {
		createChannelSelectAutoGroupsChannel(t, db, id, "default", modelName)
	}
	model.InitChannelCache()
	body := `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]}]}`
	request := func() *RetryParam {
		param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/chat/completions", modelName, "default", body)
		param.Ctx.Request.Header.Set("X-Session", t.Name())
		return param
	}
	seed := request()
	GetPreferredChannelByAffinity(seed.Ctx, modelName, "default")
	seed.Ctx.Set("channel_id", 996)
	RecordChannelAffinity(seed.Ctx, 996)
	id, found := GetPreferredChannelByAffinity(request().Ctx, modelName, "default")
	require.True(t, found)
	require.Equal(t, 996, id)

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 996).Update("status", common.ChannelStatusManuallyDisabled).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 996).Update("enabled", false).Error)
	model.InitChannelCache()
	param := request()
	channel, _, selectErr := SelectChannelForRequest(param.Ctx, modelName, param)
	require.Nil(t, selectErr)
	require.NotNil(t, channel)
	assert.Equal(t, 997, channel.Id)
	assert.Empty(t, failoverRequestFrom(param.Ctx).content.kind, "the image request stays outside the verified scope")
}

// Failover excludes and orders candidates; it never fails a request that
// ordinary routing could still serve.
func TestFailoverWithoutEligibleBackupKeepsOrdinaryRouting(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupDefaultAffinityTest(t, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
		"channel_affinity_setting.failover_mode":        "enabled",
		"channel_affinity_setting.failover_channel_ids": "[998]",
	}))
	const modelName = "single-channel-failover-regression"
	createChannelSelectAutoGroupsChannel(t, db, 998, "default", modelName)
	model.InitChannelCache()
	channel, err := model.CacheGetChannel(998)
	require.NoError(t, err)
	scope := channelFailoverScope(channel, modelName, "chat", -1)
	selectWith := func(state string) *channelFailoverRequest {
		_, _, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, _ time.Time) bool {
			next.State, next.NextCheck, next.Lease, next.LeaseUntil = state, time.Time{}, "", time.Time{}
			return true
		})
		require.NoError(t, err)
		param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/chat/completions", modelName, "default", `{"messages":[{"role":"user","content":"hello"}]}`)
		selected, _, selectErr := SelectChannelForRequest(param.Ctx, modelName, param)
		require.Nil(t, selectErr)
		require.NotNil(t, selected)
		assert.Equal(t, 998, selected.Id)
		return failoverRequestFrom(param.Ctx)
	}
	assert.Empty(t, selectWith("cooldown").trialLease, "the cooling channel still serves the request")
	assert.NotEmpty(t, selectWith("trial_ready").trialLease, "a recovered channel's ordinary attempt is its trial")
}

func TestLayeredAffinityKeepsAPIKeyPreferenceWhenUserReadFails(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupDefaultAffinityTest(t, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{
		"channel_affinity_setting.enabled":          "true",
		"channel_affinity_setting.layered_affinity": "true",
	}))
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	server.Close()
	failing := cachex.NewHybridCache(cachex.HybridCacheConfig[int]{
		Namespace: "user-read-failure-test", Redis: client, RedisEnabled: func() bool { return true },
		RedisCodec: cachex.IntCodec{}, RedisOpTimeout: 10 * time.Millisecond,
	})
	previousUsers := userAffinityCache
	userAffinityCache = func() *cachex.HybridCache[int] { return failing }
	t.Cleanup(func() { userAffinityCache = previousUsers })

	prepare := func() *RetryParam {
		param := newDefaultAffinityTestRequest(t, http.MethodPost, "/v1/chat/completions", "layered-read-regression", "default", `{"messages":[{"role":"user","content":"hello"}]}`)
		initializeLayeredAffinity(param)
		require.True(t, prepareLayeredAffinity(param))
		return param
	}
	tokenKey := layeredAffinityFrom(prepare().Ctx).token.key
	require.NoError(t, defaultAffinityCache().SetWithTTL(tokenKey, 42, time.Minute))
	param := prepare()
	preference, found := layeredAffinityPreference(param)
	require.True(t, found)
	assert.Equal(t, 42, preference.ChannelID, "a failed user read keeps the API Key preference")
	assert.Zero(t, preference.FallbackChannelID)
	require.True(t, RecordLayeredAffinityAccepted(param.Ctx, 43))
	id, _, err := defaultAffinityCache().Get(tokenKey)
	require.NoError(t, err)
	assert.Equal(t, 42, id, "an uncertain read still skips binding writes")
}

func TestChannelFailoverRetryGuardOnlyTakesOverRolloutFailures(t *testing.T) {
	rollout, outside := &model.Channel{Id: 20}, &model.Channel{Id: 21}
	for _, tc := range []struct {
		name       string
		kind       string
		owner      int
		failure    *model.Channel
		supplement bool
		session    string
		status     int
		stop       string
	}{
		{"unverified content on rollout channel retries natively", "", 0, rollout, false, "", 502, ""},
		{"unverified content outside rollout retries natively", "", 0, outside, false, "", 502, ""},
		{"verified content outside rollout retries natively", failoverText, 0, outside, false, "", 502, ""},
		{"trusted continuation keeps its original channel", "", 20, rollout, false, "", 502, "original_channel_required"},
		{"prefer session keeps its budget after a failed supplement", failoverText, 0, outside, true, "prefer", 502, ""},
		{"strict session stops after its one supplement", failoverText, 0, outside, true, "strict", 502, "failover_attempt_used"},
		{"rejected replay stops", failoverText, 0, outside, true, "prefer", 400, "migration_rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			RequestPolicy(c).SessionMode = tc.session
			c.Set(channelFailoverRequestKey, &channelFailoverRequest{
				mode: "enabled", rollout: []int{20}, content: failoverRequestContent{kind: tc.kind},
				owner: responseChannelOwner{ChannelID: tc.owner}, channel: tc.failure, failure: tc.failure,
				supplement: tc.supplement, http: channelAlertHTTP{status: tc.status},
			})
			strict, decision := channelFailoverRetryGuard(c, types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, tc.status), 5)
			assert.False(t, strict)
			if tc.stop == "" {
				assert.Nil(t, decision, "the existing retry decision applies")
				return
			}
			require.NotNil(t, decision)
			assert.Equal(t, tc.stop, decision.Reason)
		})
	}
}

func TestLayeredAffinityExcludesGeminiUpstreamReferences(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		eligible   bool
	}{
		{"text", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, true},
		{"cached content", `{"cachedContent":"cachedContents/abc","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, false},
		{"uploaded file", `{"contents":[{"role":"user","parts":[{"fileData":{"mimeType":"application/pdf","fileUri":"https://generativelanguage.googleapis.com/v1beta/files/abc"}}]}]}`, false},
		{"snake case file", `{"contents":[{"role":"user","parts":[{"file_data":{"file_uri":"files/abc"}}]}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/v1beta/models/gemini-2.5-flash:generateContent"
			param := newDefaultAffinityTestRequest(t, http.MethodPost, path, "gemini-2.5-flash", "default", tc.body)
			assert.Equal(t, tc.eligible, layeredAffinityEligible(param.Ctx, path))
		})
	}
}

func TestObserveModeResumesSamplingAfterCooldown(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previous := model.CurrentRequestPolicy().Options
	t.Cleanup(func() { require.NoError(t, model.UpdateRequestPolicyOptions(previous)) })
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{"channel_affinity_setting.failover_mode": "observe"}))
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })

	scope := ChannelFailoverScope{ChannelID: 782, Revision: "observe-v1", Model: "text", Operation: "chat", Slot: -1}
	_, _, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, now time.Time) bool {
		next.State, next.NextCheck = "cooldown", now.Add(-time.Second)
		return true
	})
	require.NoError(t, err)
	r := &channelFailoverRequest{mode: "observe"}
	expired := readChannelFailoverHealth(t.Context(), []ChannelFailoverScope{scope})[scope]
	assert.False(t, r.routingBypass(expired, time.Now()), "observe mode does not report an expired cooldown as a bypass")
	assert.True(t, (&channelFailoverRequest{mode: "enabled"}).routingBypass(expired, time.Now()), "enforced mode waits for a probe")

	health, err := observeChannelFailoverHealth(scope, http.StatusOK)
	require.NoError(t, err)
	assert.Empty(t, health.State, "a healthy result after cooldown clears the stale observe state")
}
