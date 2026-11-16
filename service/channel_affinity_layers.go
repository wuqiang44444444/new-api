package service

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const layeredAffinityRequestKey = "layered_channel_affinity"

type affinityBinding struct {
	key        string
	id         int
	readFailed bool
}

type layeredAffinityRequest struct {
	written   bool
	session   affinityBinding
	token     affinityBinding
	user      affinityBinding
	readError bool
	stream    defaultAffinityRequest
	initial   int
}

var userAffinityCache = sync.OnceValue(func() *cachex.HybridCache[int] {
	return cachex.NewInspectableHybridCache(cachex.HybridCacheConfig[int]{
		Namespace: "new-api:user_channel_affinity:v1", Redis: common.RDB, RedisEnabled: func() bool { return common.RedisEnabled },
		RedisCodec: cachex.IntCodec{}, RedisOpTimeout: defaultAffinityRedisLimit,
	}, func() *hot.HotCache[string, cachex.InspectableValue[int]] {
		capacity := operation_setting.GetChannelAffinitySetting().MaxEntries
		if capacity <= 0 {
			capacity = 100000
		}
		return hot.NewHotCache[string, cachex.InspectableValue[int]](hot.LRU, capacity).WithTTL(defaultAffinityTTL).WithJanitor().Build()
	})
})

func layeredAffinityFrom(c *gin.Context) *layeredAffinityRequest {
	raw, _ := c.Get(layeredAffinityRequestKey)
	state, _ := raw.(*layeredAffinityRequest)
	return state
}

func rememberSessionBinding(c *gin.Context, id int, err error, failoverException bool) {
	rememberFailoverBinding(c, id, err, failoverException)
	if state := layeredAffinityFrom(c); state != nil {
		state.session.id = id
		state.readError = state.readError || err != nil
	}
}

func markAffinityBindingCleared(c *gin.Context, cleared bool) {
	if !cleared {
		return
	}
	if r := failoverRequestFrom(c); r != nil {
		r.boundID = 0
	}
	if state := layeredAffinityFrom(c); state != nil {
		state.session.id = 0
	}
}

func layeredAffinityEligible(c *gin.Context, path string) bool {
	if _, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned {
		return false
	}
	if _, plugin := c.Get(jsplugin.ContextKeyPinnedEndpoint); plugin || c.GetString("expected_task_plugin_key") != "" {
		// The existing pinned generation and identity filter still own the
		// candidate set for creation on a shared protocol endpoint.
		if c.Request.Method != http.MethodPost || c.GetString("task_id") != "" {
			return false
		}
	}
	switch path {
	case "/v1/responses", "/v1/responses/compact":
		return isSelfContainedAffinityRequest(c, "/v1/responses") && c.GetHeader("X-Codex-Turn-State") == ""
	case "/v1/chat/completions", "/v1/messages":
		return c.GetHeader("X-Codex-Turn-State") == "" && isSelfContainedAffinityRequest(c, path)
	case "/v1/realtime":
		return c.Request.Method == http.MethodGet
	case "/v1/completions", "/v1/embeddings", "/v1/rerank", "/v1/moderations", "/v1/alpha/search", "/v1/audio/speech",
		"/v1/audio/transcriptions", "/v1/audio/translations", "/v1/images/generations", "/v1/images/edits", "/v1/edits",
		"/v1/videos", "/v1/video/generations", "/kling/v1/videos/text2video", "/kling/v1/videos/image2video", "/jimeng/":
		return c.Request.Method == http.MethodPost && c.GetString("task_id") == ""
	default:
		if c.Request.Method != http.MethodPost {
			return false
		}
		if isNativeGeminiAffinityPath(path) {
			return geminiAffinitySelfContained(c)
		}
		for _, action := range []string{"imagine", "describe", "blend", "video"} {
			if strings.HasSuffix(path, "/mj/submit/"+action) {
				return true
			}
		}
		return false
	}
}

func isNativeGeminiAffinityPath(path string) bool {
	return (strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/")) &&
		(strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent") ||
			strings.HasSuffix(path, ":embedContent") || strings.HasSuffix(path, ":batchEmbedContents"))
}

// Gemini requests that reference cached contents or uploaded files depend on
// one upstream account. They keep native routing instead of a lower-tier binding.
func geminiAffinitySelfContained(c *gin.Context) bool {
	storage, err := common.GetBodyStorage(c)
	if err != nil || storage.IsDisk() || storage.Size() > maxDefaultAffinityInspectionBytes {
		return false
	}
	body, err := storage.Bytes()
	return err == nil && gjson.ValidBytes(body) && !geminiUpstreamReference(gjson.ParseBytes(body))
}

func geminiUpstreamReference(value gjson.Result) bool {
	found := false
	if value.IsObject() || value.IsArray() {
		value.ForEach(func(key, item gjson.Result) bool {
			switch key.Str {
			case "cachedContent", "cached_content", "fileData", "file_data", "fileUri", "file_uri":
				found = failoverValuePresent(item)
			}
			found = found || geminiUpstreamReference(item)
			return !found
		})
	}
	return found
}

// Read lower tiers only when the session is missing or unusable. The same
// authenticated routing scope is part of both lower-tier keys.
func prepareLayeredAffinity(param *RetryParam) bool {
	c := param.Ctx
	if c == nil || c.Request == nil {
		return false
	}
	settings := model.CurrentRequestPolicy().Affinity
	if !settings.LayeredAffinity || !settings.Enabled {
		return false
	}
	if param.GetRetry() != 0 || !layeredAffinityEligible(c, c.Request.URL.Path) {
		return true
	}
	state := layeredAffinityFrom(c)
	if state == nil {
		state = &layeredAffinityRequest{}
		c.Set(layeredAffinityRequestKey, state)
	}
	if RequestPolicy(c).SessionMode == "off" {
		state.readError = true
		return true
	}
	if key, _, found := getChannelAffinityContext(c); found {
		state.session.key = key
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	scope := defaultAffinityScope{TokenID: tokenID, Model: param.ModelName, Path: c.Request.URL.Path}
	switch contract := ActiveCustomerContract(c); {
	case contract != nil:
		scope.ContractID, scope.ContractVersion = contract.Id, contract.Version
	case param.TokenGroup == "auto":
		scope.Groups = GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	default:
		scope.Groups = []string{param.TokenGroup}
	}
	if tokenID > 0 {
		state.token.key = affinityScopeDigest(scope)
		state.token.id, state.token.readFailed = readAffinityBinding(defaultAffinityCache(), state.token.key)
	}
	if userID > 0 {
		scope.TokenID = userID
		state.user.key = affinityScopeDigest(scope)
		state.user.id, state.user.readFailed = readAffinityBinding(userAffinityCache(), state.user.key)
	}
	return true
}

// Layered affinity scopes session keys by the authenticated user and route so
// equal client session values cannot share a binding across users.
func layeredSessionCacheKey(c *gin.Context, rule operation_setting.ChannelAffinityRule, modelName, group, original string) string {
	user := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if layeredAffinityFrom(c) == nil || user <= 0 {
		return original
	}
	scope := []any{user, c.Request.URL.Path, modelName, group, original}
	if contract := ActiveCustomerContract(c); contract != nil {
		scope = append(scope, contract.Id, contract.Version)
	}
	raw, _ := common.Marshal(scope)
	digest := sha256.Sum256(raw)
	return buildChannelAffinityCacheKeySuffix(rule, modelName, group, hex.EncodeToString(digest[:]))
}

func affinityScopeDigest(scope defaultAffinityScope) string {
	encoded, _ := common.Marshal(scope)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func readAffinityBinding(cache *cachex.HybridCache[int], key string) (int, bool) {
	id, _, err := cache.Get(key)
	if err != nil {
		logDefaultAffinityCacheFailure("layer read")
	}
	return id, err != nil
}

// A failed read only drops that tier and the tiers below it: an unknown
// API Key binding cannot be replaced by the user binding.
func layeredAffinityPreference(param *RetryParam) (*model.ChannelSelectionPreference, bool) {
	state := layeredAffinityFrom(param.Ctx)
	if state == nil {
		return nil, false
	}
	preference := &model.ChannelSelectionPreference{}
	if param.GetRetry() == 0 && state.initial == 0 && !state.readError && !state.token.readFailed {
		preference.ChannelID = state.token.id
		if !state.user.readFailed {
			preference.FallbackChannelID = state.user.id
		}
	}
	return preference, true
}

func affinityStreamTracker(c *gin.Context) *defaultAffinityRequest {
	if state := layeredAffinityFrom(c); state != nil {
		return &state.stream
	}
	if state := failoverRequestFrom(c); state != nil && state.mode == "enabled" {
		return &state.stream
	}
	state, _ := c.Get(defaultAffinityContextKey)
	affinity, _ := state.(*defaultAffinityRequest)
	return affinity
}

func OwnsNativeChannelAffinityWrite(c *gin.Context) bool {
	if r := failoverRequestFrom(c); r != nil && r.mode == "enabled" {
		return true
	}
	return layeredAffinityFrom(c) != nil && c.Request.Method == http.MethodPost
}

func affinityTTL(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = model.CurrentRequestPolicy().Affinity.DefaultTTLSeconds
	}
	if seconds <= 0 || int64(seconds) > math.MaxInt64/int64(time.Second) {
		return defaultAffinityTTL
	}
	return time.Duration(seconds) * time.Second
}

func storeAffinityBinding(c *gin.Context, cache *cachex.HybridCache[int], binding affinityBinding, channelID int, ttl time.Duration, maySwitch bool) {
	if binding.key == "" || (binding.id != 0 && binding.id != channelID && !maySwitch) {
		return
	}
	var expected *int
	if binding.id > 0 {
		expected = &binding.id
	}
	ok, err := cache.CompareAndSwapWithTTL(binding.key, expected, channelID, ttl)
	if err != nil {
		logDefaultAffinityCacheFailure("conditional write")
	} else if ok && binding.id > 0 && binding.id != channelID {
		routingEvent(c, channelID, "rebind", "binding_updated_after_success")
	}
}

func RecordNativeChannelAffinitySuccess(c *gin.Context, channelID int, info *relaycommon.RelayInfo) {
	// The failover and layered gates come from the request policy snapshot,
	// so the write path reads the same validated snapshot.
	settings := model.CurrentRequestPolicy().Affinity
	r := failoverRequestFrom(c)
	layers := layeredAffinityFrom(c)
	if layers != nil && layers.written || r != nil && r.affinityWritten {
		return
	}
	if r != nil && r.mode == "enabled" && r.owner.ChannelID != 0 {
		return
	}
	if layers != nil {
		if !channelRelayCompleted(c, &layers.stream, info) {
			return
		}
		layers.written = true
		recordLayeredAffinitySuccess(c, channelID)
		return
	}
	if r == nil || r.mode != "enabled" {
		RecordDefaultChannelAffinitySuccess(c, channelID, info)
		return
	}
	if !settings.Enabled || !channelRelayCompleted(c, &r.stream, info) {
		return
	}
	if RequestPolicy(c).SessionMode == "off" {
		return
	}
	r.affinityWritten = true
	if key, ttl, found := getChannelAffinityContext(c); found {
		if !r.boundReadError {
			storeAffinityBinding(c, getChannelAffinityCache(), affinityBinding{key: key, id: r.boundID}, channelID, affinityTTL(ttl), settings.SwitchOnSuccess)
		}
		return
	}
	raw, _ := c.Get(defaultAffinityContextKey)
	affinity, _ := raw.(*defaultAffinityRequest)
	if affinity != nil && !affinity.readError && affinity.current.channelID == channelID &&
		(settings.SwitchOnSuccess || affinity.first.channelID == channelID) {
		storeAffinityBinding(c, defaultAffinityCache(), affinityBinding{key: affinity.key, id: affinity.preferred}, channelID, affinityTTL(0), settings.SwitchOnSuccess)
	}
}

func recordLayeredAffinitySuccess(c *gin.Context, channelID int) {
	state := layeredAffinityFrom(c)
	if state == nil || state.readError || state.token.readFailed || state.user.readFailed || channelID <= 0 {
		return
	}
	if r := failoverRequestFrom(c); r != nil && r.boundReadError {
		return
	}
	settings := model.CurrentRequestPolicy().Affinity
	if !settings.Enabled || RequestPolicy(c).SessionMode == "off" || (!settings.SwitchOnSuccess && state.initial != 0 && state.initial != channelID) {
		return
	}
	maySwitch := settings.SwitchOnSuccess
	if key, ttl, found := getChannelAffinityContext(c); found {
		state.session.key = key
		storeAffinityBinding(c, getChannelAffinityCache(), state.session, channelID, affinityTTL(ttl), maySwitch)
		maySwitch = false
	}
	storeAffinityBinding(c, defaultAffinityCache(), state.token, channelID, affinityTTL(0), maySwitch)
	storeAffinityBinding(c, userAffinityCache(), state.user, channelID, affinityTTL(0), maySwitch && state.token.key == "")
}

func RecordLayeredAffinityChannelSelection(c *gin.Context, id int) {
	if state := layeredAffinityFrom(c); state != nil && state.initial == 0 {
		state.initial = id
	}
}

// These handlers already validate their completion or submit acceptance.
// The distributor does not use the HTTP status to write the lower tiers.
func RecordLayeredAffinityAccepted(c *gin.Context, id int) bool {
	state := layeredAffinityFrom(c)
	if state == nil {
		return false
	}
	if !state.written {
		state.written = true
		recordLayeredAffinitySuccess(c, id)
	}
	return true
}

func initializeLayeredAffinity(param *RetryParam) {
	c := param.Ctx
	if c == nil || c.Request == nil {
		return
	}
	settings := model.CurrentRequestPolicy().Affinity
	if layeredAffinityFrom(c) == nil && settings.LayeredAffinity && settings.Enabled && layeredAffinityEligible(c, c.Request.URL.Path) {
		c.Set(layeredAffinityRequestKey, &layeredAffinityRequest{})
	}
}

func FinishChannelFailoverRouting(c *gin.Context) {
	if r := failoverRequestFrom(c); r != nil && r.trialLease != "" {
		_, _ = CompleteChannelFailoverCheck(r.trialScope, r.trialLease, "unconfirmed", 0, true)
		r.trialLease = ""
	}
}

func channelFailureFallbackStatus(c *gin.Context, status int) int {
	if status == 0 {
		return clienterrlog.UpstreamResponseStatus(c.Request.Context())
	}
	return status
}

func ObserveNativeStreamOutcome(c *gin.Context, info *relaycommon.RelayInfo) {
	// The log path and the relay success path both observe the outcome; record it once.
	state := RequestPolicy(c)
	if state.OutcomeRecorded {
		return
	}
	if tracker := affinityStreamTracker(c); OwnsNativeChannelAffinityWrite(c) && tracker != nil && !channelRelayCompleted(c, tracker, info) {
		state.OutcomeRecorded = true
		routingEvent(c, c.GetInt("channel_id"), "stop", "stream_not_successful")
	}
}

func ObserveNativeAffinityLogOutcome(c *gin.Context, info *relaycommon.RelayInfo) {
	ObserveNativeStreamOutcome(c, info)
	observeChannelFailoverEligibility(c)
	if OwnsNativeChannelAffinityWrite(c) {
		RecordNativeChannelAffinitySuccess(c, info.GetChannelID(), info)
	}
}
