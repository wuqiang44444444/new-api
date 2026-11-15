package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/samber/hot"
)

const (
	failoverObservationWindow = time.Minute
	failoverStateTTL          = 24 * time.Hour
	failoverLeaseTime         = 2 * time.Minute
	failoverRedisTimeout      = 10 * time.Millisecond
	failoverNamespace         = "new-api:channel_failover:v1"
)

type ChannelFailoverScope struct {
	ChannelID int    `json:"channel_id"`
	Revision  string `json:"revision"`
	Model     string `json:"model"`
	Operation string `json:"operation"`
	Slot      int    `json:"slot"`
}

func (s ChannelFailoverScope) key() string {
	data, _ := common.Marshal(s)
	return base64.RawURLEncoding.EncodeToString(data)
}

type channelFailureSample struct {
	At     time.Time `json:"at"`
	Status int       `json:"status"`
}

type ChannelFailoverHealth struct {
	Scope        ChannelFailoverScope   `json:"scope"`
	Samples      []channelFailureSample `json:"samples,omitempty"`
	State        string                 `json:"state"`
	NextCheck    time.Time              `json:"next_check"`
	ProbeResult  string                 `json:"probe_result,omitempty"`
	ProbeStatus  int                    `json:"probe_status,omitempty"`
	TrialResult  string                 `json:"trial_result,omitempty"`
	TrialStatus  int                    `json:"trial_status,omitempty"`
	UpdatedAt    time.Time              `json:"updated_at"`
	ManualUntil  time.Time              `json:"manual_until"`
	ManualReason string                 `json:"manual_reason,omitempty"`
	CooldownStep int                    `json:"cooldown_step,omitempty"`
	Lease        string                 `json:"lease,omitempty"`
	LeaseUntil   time.Time              `json:"lease_until"`
}

var sharedFailoverHealth = sync.OnceValue(func() *cachex.HybridCache[ChannelFailoverHealth] {
	return cachex.NewHybridCache(cachex.HybridCacheConfig[ChannelFailoverHealth]{
		Namespace: failoverNamespace, Redis: common.RDB, RedisEnabled: func() bool { return common.RedisEnabled },
		RedisCodec: cachex.JSONCodec[ChannelFailoverHealth]{}, RedisOpTimeout: failoverRedisTimeout,
		Memory: func() *hot.HotCache[string, ChannelFailoverHealth] {
			return hot.NewHotCache[string, ChannelFailoverHealth](hot.LRU, 10000).WithTTL(failoverStateTTL).WithJanitor().Build()
		},
	})
})

var localFailoverReads = hot.NewHotCache[string, ChannelFailoverHealth](hot.LRU, 10000).WithTTL(time.Second).WithJanitor().Build()
var localFailoverManual = hot.NewHotCache[string, ChannelFailoverHealth](hot.LRU, 10000).WithTTL(failoverStateTTL).WithJanitor().Build()

func ChannelFailoverConfigured() bool {
	mode := model.CurrentRequestPolicy().Affinity.FailoverMode
	return mode == "observe" || mode == "enabled"
}

func failoverScopeAllowed(scope ChannelFailoverScope, settings operation_setting.ChannelAffinitySetting) bool {
	return (settings.FailoverMode == "observe" && len(settings.FailoverChannelIDs) == 0) ||
		slices.Contains(settings.FailoverChannelIDs, scope.ChannelID)
}

func ChannelFailoverOperation(path string) string {
	switch path {
	case "/v1/messages":
		return "messages"
	case "/v1/chat/completions":
		return "chat"
	case "/v1/responses":
		return "responses"
	default:
		return ""
	}
}

func channelFailoverScope(channel *model.Channel, modelName, operation string, slot int) ChannelFailoverScope {
	return ChannelFailoverScope{ChannelID: channel.Id, Revision: model.ChannelAlertRevision(channel), Model: modelName, Operation: operation, Slot: slot}
}

func readChannelFailoverHealth(ctx context.Context, scopes []ChannelFailoverScope) map[ChannelFailoverScope]ChannelFailoverHealth {
	states := make(map[ChannelFailoverScope]ChannelFailoverHealth, len(scopes))
	var missing []ChannelFailoverScope
	var keys []string
	for _, scope := range scopes {
		key := scope.key()
		if value, found, _ := localFailoverReads.Get(key); found {
			states[scope] = value
		} else if common.RedisEnabled && common.RDB != nil {
			missing = append(missing, scope)
			keys = append(keys, sharedFailoverHealth().FullKey(key))
		} else if value, _, err := sharedFailoverHealth().Get(key); err == nil {
			states[scope] = value
		}
	}
	if len(keys) == 0 {
		return states
	}
	ctx, cancel := context.WithTimeout(ctx, failoverRedisTimeout)
	defer cancel()
	values, err := common.RDB.MGet(ctx, keys...).Result()
	for i, scope := range missing {
		key := scope.key()
		var value ChannelFailoverHealth
		if err != nil {
			value, _, _ = localFailoverManual.Get(key)
			if value.ManualUntil.After(time.Now()) {
				states[scope] = value
			}
			continue
		}
		if raw, ok := values[i].(string); ok && common.UnmarshalJsonStr(raw, &value) != nil {
			continue
		}
		localFailoverReads.Set(key, value)
		cacheFailoverManual(key, value)
		states[scope] = value
	}
	return states
}

func cacheFailoverManual(key string, value ChannelFailoverHealth) {
	if remaining := time.Until(value.ManualUntil); remaining > 0 {
		localFailoverManual.SetWithTTL(key, ChannelFailoverHealth{ManualUntil: value.ManualUntil}, remaining)
	} else {
		localFailoverManual.Delete(key)
	}
}

func updateChannelFailoverHealth(scope ChannelFailoverScope, update func(*ChannelFailoverHealth, time.Time) bool) (ChannelFailoverHealth, bool, error) {
	cache := sharedFailoverHealth()
	key := scope.key()
	for range 6 {
		current, found, err := cache.Get(key)
		if err != nil {
			return current, false, err
		}
		next := current
		next.Scope = scope
		now := time.Now()
		if !update(&next, now) {
			return current, false, nil
		}
		next.UpdatedAt = now
		var previous *ChannelFailoverHealth
		if found {
			previous = &current
		}
		swapped, err := cache.CompareAndSwapWithTTL(key, previous, next, failoverStateTTL)
		if err != nil {
			return current, false, err
		}
		if swapped {
			localFailoverReads.Delete(key)
			cacheFailoverManual(key, next)
			return next, true, nil
		}
	}
	return ChannelFailoverHealth{}, false, fmt.Errorf("channel health changed concurrently")
}

func qualifiesChannelFailure(status int) bool {
	switch status {
	case 0:
		return true // A connection failure independently observed by the transport.
	case http.StatusBadGateway, http.StatusServiceUnavailable, 529:
		return operation_setting.ShouldRetryByStatusCode(status)
	default:
		return false
	}
}

func observeChannelFailoverHealth(scope ChannelFailoverScope, status int) (ChannelFailoverHealth, error) {
	observeOnly := model.CurrentRequestPolicy().Affinity.FailoverMode == "observe"
	value, _, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, now time.Time) bool {
		// Observe mode has no probe, so sampling resumes once cooldown expires.
		expired := observeOnly && next.State == "cooldown" && next.Lease == "" && !next.NextCheck.After(now)
		if expired {
			next.State, next.Samples, next.NextCheck, next.CooldownStep = "", nil, time.Time{}, 0
		}
		if next.State != "" || next.Lease != "" {
			return false
		}
		if !qualifiesChannelFailure(status) {
			if len(next.Samples) == 0 {
				return expired
			}
			next.Samples = nil
			return true
		}
		samples := make([]channelFailureSample, 0, 3)
		for _, sample := range next.Samples {
			if now.Sub(sample.At) >= failoverObservationWindow || !qualifiesChannelFailure(sample.Status) {
				// A sample outside the current retry policy breaks the streak.
				samples = samples[:0]
				continue
			}
			samples = append(samples, sample)
		}
		next.Samples = append(samples, channelFailureSample{At: now, Status: status})
		if len(next.Samples) >= 3 {
			next.State = "cooldown"
			next.NextCheck = now.Add(time.Minute)
		}
		return true
	})
	return value, err
}

func (s ChannelFailoverHealth) bypassed(now time.Time) bool {
	return s.State != "" || s.ManualUntil.After(now)
}

func (s *ChannelFailoverHealth) coolDown(now time.Time) {
	s.CooldownStep = min(s.CooldownStep+1, 5)
	s.State = "cooldown"
	s.NextCheck = now.Add(time.Minute << s.CooldownStep)
}

func (s ChannelFailoverHealth) trialReady(now time.Time) bool {
	return s.State == "trial_ready" && !s.LeaseUntil.After(now) && !s.ManualUntil.After(now)
}

// ClaimChannelFailoverProbe reserves the same slot used by live trials. The
// lease token fences late results from workers whose lease has expired.
func ClaimChannelFailoverProbe(scope ChannelFailoverScope) (string, bool, error) {
	return claimChannelFailoverLease(scope, false)
}

func claimChannelFailoverLease(scope ChannelFailoverScope, trial bool) (string, bool, error) {
	if !ChannelFailoverConfigured() || !failoverScopeAllowed(scope, model.CurrentRequestPolicy().Affinity) {
		return "", false, nil
	}
	token := common.NewRequestId()
	_, granted, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, now time.Time) bool {
		ready := next.State == "cooldown" || next.State == "pending"
		if trial {
			ready = next.trialReady(now)
		}
		if !ready || next.NextCheck.After(now) || next.LeaseUntil.After(now) || next.ManualUntil.After(now) {
			return false
		}
		next.Lease = token
		next.LeaseUntil = now.Add(failoverLeaseTime)
		return true
	})
	return token, granted, err
}

// CompleteChannelFailoverCheck reports whether the result was committed. An
// expired or replaced lease leaves the shared state unchanged.
func CompleteChannelFailoverCheck(scope ChannelFailoverScope, lease, result string, status int, trial bool) (bool, error) {
	_, committed, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, now time.Time) bool {
		if lease == "" || lease != next.Lease || !next.LeaseUntil.After(now) {
			return false
		}
		next.Lease = ""
		next.LeaseUntil = time.Time{}
		if trial {
			next.TrialResult, next.TrialStatus = result, status
		} else {
			next.ProbeResult, next.ProbeStatus = result, status
		}
		switch {
		case trial && result == "success":
			next.State, next.Samples, next.CooldownStep, next.NextCheck = "", nil, 0, time.Time{}
		case trial && result == "unconfirmed":
			// An interrupted trial is not failure evidence; the slot opens again.
		case result == "success" || result == "manual_trial":
			next.State, next.NextCheck = "trial_ready", time.Time{}
		case result == "failed" || trial:
			next.coolDown(now)
		default:
			next.State, next.NextCheck = "pending", now.Add(time.Minute)
		}
		return true
	})
	return committed, err
}

func SetChannelFailoverBypass(scope ChannelFailoverScope, duration time.Duration, reason string) error {
	_, _, err := updateChannelFailoverHealth(scope, func(next *ChannelFailoverHealth, now time.Time) bool {
		next.ManualUntil, next.ManualReason = time.Time{}, ""
		if duration > 0 {
			next.ManualUntil, next.ManualReason = now.Add(duration), reason
		}
		return true
	})
	return err
}

// ListChannelFailoverHealth is an operator path. The request path only reads
// exact candidate keys, with one shared deadline for the batch.
func ListChannelFailoverHealth(ctx context.Context, channelID int) ([]ChannelFailoverHealth, error) {
	cache := sharedFailoverHealth()
	keys, err := cache.Keys()
	if err != nil {
		return nil, err
	}
	rows := make([]ChannelFailoverHealth, 0)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var scope ChannelFailoverScope
		encoded, namespaced := strings.CutPrefix(key, failoverNamespace+":")
		if !namespaced {
			continue
		}
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || common.Unmarshal(data, &scope) != nil {
			continue
		}
		if scope.ChannelID == 0 || channelID != 0 && scope.ChannelID != channelID {
			continue
		}
		state, found, err := cache.Get(key)
		if err != nil {
			return nil, err
		}
		if found {
			rows = append(rows, state)
		}
	}
	slices.SortFunc(rows, func(a, b ChannelFailoverHealth) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return rows, nil
}

func ChannelFailoverScopeConfigured(scope ChannelFailoverScope) bool {
	return ChannelFailoverConfigured() && failoverScopeAllowed(scope, model.CurrentRequestPolicy().Affinity)
}

func IsChannelFailoverProbeFailure(status int) bool {
	return status > 0 && qualifiesChannelFailure(status)
}
