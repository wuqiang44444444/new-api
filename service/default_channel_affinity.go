package service

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
)

const (
	defaultAffinityContextKey = "default_channel_affinity"
	defaultAffinityNamespace  = "new-api:default_channel_affinity:v1"
	defaultAffinityCapacity   = 10_000
	defaultAffinityRedisLimit = 10 * time.Millisecond
	defaultAffinityTTL        = time.Hour
)

var defaultAffinityCache = sync.OnceValue(func() *cachex.HybridCache[int] {
	return newDefaultAffinityCache(common.RDB)
})

func newDefaultAffinityCache(redisClient *redis.Client) *cachex.HybridCache[int] {
	return cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
		Namespace:      cachex.Namespace(defaultAffinityNamespace),
		Redis:          redisClient,
		RedisEnabled:   func() bool { return common.RedisEnabled },
		RedisCodec:     cachex.IntCodec{},
		RedisOpTimeout: defaultAffinityRedisLimit,
		Memory: func() *hot.HotCache[string, int] {
			return hot.NewHotCache[string, int](hot.LRU, defaultAffinityCapacity).WithTTL(defaultAffinityTTL).WithJanitor().Build()
		},
	})
}

// Redis entry counts are measured separately with an operational SCAN. An
// admin page refresh must not synchronously scan the new shared namespace.
func defaultAffinityLocalEntryCount() *int {
	if common.RedisEnabled && common.RDB != nil {
		return nil
	}
	keys, _ := defaultAffinityCache().Keys()
	return common.GetPointer(len(keys))
}

var defaultAffinityLogAfter atomic.Int64

type defaultAffinityScope struct {
	TokenID         int
	Model           string
	Path            string
	Groups          []string
	ContractID      int
	ContractVersion int64
}

type defaultAffinitySelection struct {
	channelID int
	group     string
	priority  int64
}

type defaultAffinityRequest struct {
	key                 string
	preferred           int
	readError           bool
	contract            bool
	first               defaultAffinitySelection
	current             defaultAffinitySelection
	chatOverrideAttempt *relaycommon.ChannelMeta
	chatOverrideChoices int
	chatChoiceStream    *relaycommon.StreamStatus
	finishedChatChoices map[int]struct{}
}

func prepareDefaultChannelAffinity(param *RetryParam) {
	c := param.Ctx
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost || param.GetRetry() != 0 ||
		!operation_setting.GetChannelAffinitySetting().Enabled {
		return
	}
	path := c.Request.URL.Path
	switch path {
	case "/v1/chat/completions", "/v1/messages", "/v1/responses":
	default:
		return
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenID <= 0 || param.ModelName == "" {
		return
	}
	_, matched := getChannelAffinityMeta(c)
	reason := ""
	if _, pinned := c.Get(jsplugin.ContextKeyPinnedEndpoint); pinned || c.GetString("expected_task_plugin_key") != "" {
		reason = "plugin_route"
	} else if matched {
		reason = "session_rule_matched"
	} else if !isSelfContainedAffinityRequest(c, path) {
		reason = "request_outside_scope"
	}
	if reason != "" {
		RequestPolicy(c).AddEvent(PolicyEvent{Decision: PolicyDecision{Action: "skip", Reason: reason, Source: defaultAffinityContextKey}})
		return
	}

	scope := defaultAffinityScope{TokenID: tokenID, Model: param.ModelName, Path: path}
	switch contract := ActiveCustomerContract(c); {
	case contract != nil:
		scope.ContractID, scope.ContractVersion = contract.Id, contract.Version
	case param.TokenGroup == "auto":
		scope.Groups = GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
		if len(scope.Groups) == 0 {
			return
		}
	default:
		scope.Groups = []string{param.TokenGroup}
	}
	encoded, err := common.Marshal(scope)
	if err != nil {
		return
	}
	digest := sha256.Sum256(encoded)
	state := &defaultAffinityRequest{key: hex.EncodeToString(digest[:]), contract: scope.ContractID > 0}
	if id, found, err := defaultAffinityCache().Get(state.key); err != nil {
		state.readError = true
		logDefaultAffinityCacheFailure("read")
	} else if found && id > 0 {
		state.preferred = id
	}
	c.Set(defaultAffinityContextKey, state)
}

func selectChannelWithDefaultAffinity(param *RetryParam, group string, retry int, filters []dto.ChannelFilter) (*model.Channel, error) {
	state, _ := param.Ctx.Get(defaultAffinityContextKey)
	affinity, ok := state.(*defaultAffinityRequest)
	if !ok {
		return model.GetRandomSatisfiedChannel(group, param.ModelName, retry, filters)
	}
	preference := model.ChannelSelectionPreference{}
	initial := affinity.first.channelID == 0
	if initial {
		preference.ChannelID = affinity.preferred
	}
	channel, err := model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, &preference)
	if err != nil || channel == nil {
		return channel, err
	}
	if affinity.contract {
		for _, filter := range filters {
			if filter.Kind == dto.FilterContractRoutes {
				group = filter.Routes[channel.Id]
				break
			}
		}
	}
	affinity.current = defaultAffinitySelection{channelID: channel.Id, group: group, priority: preference.SelectedPriority}
	if initial {
		affinity.first = affinity.current
		action, reason := "miss", "binding_missing"
		switch {
		case affinity.readError:
			action, reason = "skip", "cache_unavailable"
		case affinity.preferred == channel.Id && channel.Status == common.ChannelStatusEnabled:
			action, reason = "hit", "binding_used"
		case affinity.preferred > 0:
			reason = "binding_not_eligible"
		}
		RequestPolicy(param.Ctx).AddEvent(PolicyEvent{ChannelID: channel.Id, Group: group, Decision: PolicyDecision{Action: action, Reason: reason, Source: defaultAffinityContextKey}})
	}
	return channel, nil
}

// RecordDefaultChannelAffinitySuccess is called only after the native HTTP
// relay returns successfully. A stream also needs a trustworthy terminal.
func RecordDefaultChannelAffinitySuccess(c *gin.Context, channelID int, info *relaycommon.RelayInfo) {
	state, _ := c.Get(defaultAffinityContextKey)
	affinity, ok := state.(*defaultAffinityRequest)
	if !ok || affinity.readError || channelID <= 0 || channelID != affinity.current.channelID ||
		(!affinity.contract && affinity.first.group != affinity.current.group) || affinity.first.priority != affinity.current.priority {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if !setting.Enabled || (!setting.SwitchOnSuccess && channelID != affinity.first.channelID) {
		return
	}
	// The response may have been flushed already. A client that closes after
	// receiving it can cancel the HTTP context before relay accounting returns.
	if status := c.Writer.Status(); status < http.StatusOK || status >= http.StatusMultipleChoices {
		return
	}
	stream := info.StreamStatus
	if info.IsStream || stream != nil {
		if stream == nil || !stream.IsNormalEnd() || stream.HasErrors() || stream.ResponseOutcome() != string(relaycommon.ResponseOutcomeCompleted) {
			return
		}
		count := affinity.chatChoiceCount(c, info)
		if count == 0 || (count > 1 && (affinity.chatChoiceStream != stream || len(affinity.finishedChatChoices) != count)) {
			return
		}
	}
	ttl := defaultAffinityTTL
	if seconds := setting.DefaultTTLSeconds; seconds > 0 && int64(seconds) <= math.MaxInt64/int64(time.Second) {
		ttl = time.Duration(seconds) * time.Second
	}
	if err := defaultAffinityCache().SetWithTTL(affinity.key, channelID, ttl); err != nil {
		logDefaultAffinityCacheFailure("write")
	}
}

func logDefaultAffinityCacheFailure(operation string) {
	now := time.Now().Unix()
	previous := defaultAffinityLogAfter.Load()
	if now >= previous && defaultAffinityLogAfter.CompareAndSwap(previous, now+60) {
		common.SysError("default channel affinity cache " + operation + " failed")
	}
}
