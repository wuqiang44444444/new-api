package service

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const channelFailoverRequestKey = "channel_failover"

type channelFailoverRequest struct {
	nonStreamTerminal   bool
	nonStreamObserved   bool
	affinityWritten     bool
	observedEligibility bool
	mode                string
	operation           string
	tokenGroup          string
	modelName           string
	rollout             []int
	content             failoverRequestContent
	owner               responseChannelOwner
	channel             *model.Channel
	scope               ChannelFailoverScope
	group               string
	http                channelAlertHTTP
	bound               *model.Channel
	boundID             int
	boundReadError      bool
	strictException     bool
	failed              map[int]bool
	failure             *model.Channel
	failureHealth       ChannelFailoverHealth
	supplement          bool
	trialScope          ChannelFailoverScope
	trialLease          string
	stream              defaultAffinityRequest
	responseID          string
	responseTurn        string
	recoveryGroup       string
	healthDeadline      time.Time
}

func failoverRequestFrom(c *gin.Context) *channelFailoverRequest {
	if c == nil {
		return nil
	}
	raw, _ := c.Get(channelFailoverRequestKey)
	r, _ := raw.(*channelFailoverRequest)
	return r
}

func initializeFailoverRequest(param *RetryParam) *ChannelSelectError {
	c := param.Ctx
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost || failoverRequestFrom(c) != nil {
		return nil
	}
	operation := ChannelFailoverOperation(c.Request.URL.Path)
	if operation == "" || !ChannelFailoverConfigured() {
		return nil
	}
	if _, plugin := c.Get(jsplugin.ContextKeyPinnedEndpoint); plugin || c.GetString("expected_task_plugin_key") != "" {
		return nil
	}
	settings := model.CurrentRequestPolicy().Affinity
	r := &channelFailoverRequest{
		mode: settings.FailoverMode, operation: operation, rollout: settings.FailoverChannelIDs,
		modelName: param.ModelName, tokenGroup: param.TokenGroup, content: inspectFailoverContent(c, operation),
	}
	c.Set(channelFailoverRequestKey, r)
	// Unresolved upstream references keep native routing. Their content kind
	// stays empty, so failover never bypasses or supplements them.
	if r.mode == "enabled" && len(r.content.ownerReferences) > 0 {
		owner, found := resolveResponseOwner(param, r.content.ownerReferences)
		if !found || !slices.Contains(r.rollout, owner.ChannelID) {
			return nil
		}
		if pin, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned && pin.ChannelId != owner.ChannelID {
			return failoverContinuationError()
		}
		r.owner = owner
	}
	return nil
}

func failoverContinuationError() *ChannelSelectError {
	return &ChannelSelectError{StatusCode: http.StatusConflict, Code: "upstream_continuation_unavailable", Message: "upstream_continuation_unavailable"}
}

func selectOwnedResponseChannel(param *RetryParam) (*model.Channel, string, *ChannelSelectError) {
	c := param.Ctx
	r := failoverRequestFrom(c)
	channel, err := model.CacheGetChannel(r.owner.ChannelID)
	if err != nil || channel.Status != common.ChannelStatusEnabled || model.ChannelAlertRevision(channel) != r.owner.Revision {
		return nil, "", failoverContinuationError()
	}
	filters := GetChannelConstraints(c).Filters
	if contract := ActiveCustomerContract(c); contract != nil {
		routes, err := customerContractRoutes(c, param.ModelName)
		group := routes[channel.Id]
		if err != nil || group == "" {
			return nil, "", failoverContinuationError()
		}
		filters = append(slices.Clone(filters), dto.ChannelFilter{Kind: dto.FilterContractRoutes, Routes: routes})
		if ok, _ := model.ChannelSatisfiesFilters(channel, param.ModelName, filters); !ok {
			return nil, "", failoverContinuationError()
		}
		setCustomerContractGroup(c, group)
		return channel, group, nil
	}
	if ok, _ := model.ChannelSatisfiesFilters(channel, param.ModelName, filters); !ok {
		return nil, "", failoverContinuationError()
	}
	groups := []string{param.TokenGroup}
	if param.TokenGroup == "auto" {
		groups = GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	for _, group := range groups {
		if model.IsChannelEnabledForGroupModel(group, param.ModelName, channel.Id) {
			if param.TokenGroup == "auto" {
				common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
			}
			return channel, group, nil
		}
	}
	return nil, "", failoverContinuationError()
}

func rememberFailoverBinding(c *gin.Context, id int, err error, exception bool) {
	if r := failoverRequestFrom(c); r != nil {
		r.boundID, r.boundReadError, r.strictException = id, err != nil, exception
	}
}

// beginChannelFailoverSelection prepares per-request routing state. Requests
// with a trusted Responses owner are handled here and never reach affinity.
func beginChannelFailoverSelection(retry *RetryParam) (*model.Channel, string, *ChannelSelectError, bool) {
	initializeLayeredAffinity(retry)
	if selectionErr := initializeFailoverRequest(retry); selectionErr != nil {
		return nil, "", selectionErr, true
	}
	if r := failoverRequestFrom(retry.Ctx); r != nil && r.owner.ChannelID != 0 {
		channel, group, selectionErr := selectOwnedResponseChannel(retry)
		return channel, group, selectionErr, true
	}
	return nil, "", nil, false
}

func usesAutoFailoverSupplement(param *RetryParam) bool {
	r := failoverRequestFrom(param.Ctx)
	return r != nil && param.TokenGroup == "auto" && ActiveCustomerContract(param.Ctx) == nil && (r.supplement || r.bound != nil)
}

func (e *ChannelSelectError) Error() string { return e.Message }

// Observe mode never probes, so an expired cooldown no longer counts as a
// bypass there. Enforced mode keeps it until a probe or trial resolves it.
func (r *channelFailoverRequest) routingBypass(state ChannelFailoverHealth, now time.Time) bool {
	if r.mode == "observe" && state.State == "cooldown" && !state.NextCheck.After(now) && !state.ManualUntil.After(now) {
		return false
	}
	return state.bypassed(now)
}

func (r *channelFailoverRequest) inRollout(channelID int) bool {
	return len(r.rollout) == 0 && r.mode == "observe" || slices.Contains(r.rollout, channelID)
}

// failoverSource reports whether failover may move this request away from
// the channel. Every takeover decision uses this one rule.
func (r *channelFailoverRequest) failoverSource(channel *model.Channel) bool {
	return r.inRollout(channel.Id) && failoverChannelInScope(r.operation, channel) &&
		(!r.content.claudeKeepAll || r.isVerifiedClaudeContextSource(channel))
}

func channelFailoverHealthFor(r *channelFailoverRequest, c *gin.Context, channels []*model.Channel) map[int]ChannelFailoverHealth {
	scopes := make([]ChannelFailoverScope, 0, len(channels))
	for _, channel := range channels {
		// Until key selection can avoid unhealthy slots, multi-key channels
		// contribute evidence without entering automatic exclusion or trials.
		if r.inRollout(channel.Id) && !channel.ChannelInfo.IsMultiKey {
			scopes = append(scopes, channelFailoverScope(channel, r.modelName, r.operation, -1))
		}
	}
	states := make(map[int]ChannelFailoverHealth, len(scopes))
	if r.healthDeadline.IsZero() {
		r.healthDeadline = time.Now().Add(failoverRedisTimeout)
	}
	ctx, cancel := context.WithDeadline(c.Request.Context(), r.healthDeadline)
	defer cancel()
	for scope, state := range readChannelFailoverHealth(ctx, scopes) {
		states[scope.ChannelID] = state
	}
	return states
}

func routingEvent(c *gin.Context, channelID int, action, reason string) {
	RequestPolicy(c).AddEvent(PolicyEvent{ChannelID: channelID, Decision: PolicyDecision{Action: action, Reason: reason, Source: "channel_failover"}})
	if action == "rebind" || action == "trial_result" {
		logger.LogInfo(c, common.LogText("channel failover action=%s reason=%s channel_id=%d", action, reason, channelID))
	}
}

// A usable binding is checked for health only after authorization. A
// temporary bypass leaves the binding intact, including a strict binding.
func allowFailoverBinding(param *RetryParam, bound *model.Channel, group string) bool {
	c := param.Ctx
	r := failoverRequestFrom(c)
	if r == nil || bound == nil || r.content.kind == "" || len(r.content.ownerReferences) != 0 || !r.failoverSource(bound) ||
		(RequestPolicy(c).SessionMode == "strict" && !r.strictException) {
		return true
	}
	state := channelFailoverHealthFor(r, c, []*model.Channel{bound})[bound.Id]
	if !r.routingBypass(state, time.Now()) {
		return true
	}
	if r.mode == "observe" {
		routingEvent(c, bound.Id, "would_bypass", "bound_channel_would_be_bypassed")
		return true
	}
	r.bound, r.recoveryGroup = bound, group
	return false
}

// Commit a first-attempt bypass only after the authorized selector found a
// backup. The original binding already passed the caller's eligibility checks.
func resolveFailoverBinding(c *gin.Context, selected *model.Channel, group string, err error) (*model.Channel, string, error) {
	r := failoverRequestFrom(c)
	if r == nil || r.bound == nil {
		return selected, group, err
	}
	if err == nil && selected != nil {
		if selected.Id != r.bound.Id {
			routingEvent(c, r.bound.Id, "bypass", "bound_channel_temporarily_unavailable")
			routingEvent(c, selected.Id, "backup", "eligible_failover_channel")
		}
		return selected, group, nil
	}
	bound, boundGroup := r.bound, r.recoveryGroup
	r.bound, r.recoveryGroup = nil, ""
	if r.tokenGroup == "auto" && ActiveCustomerContract(c) == nil {
		common.SetContextKey(c, constant.ContextKeyAutoGroup, boundGroup)
	}
	MarkChannelAffinityUsed(c, boundGroup, bound.Id)
	routingEvent(c, bound.Id, "keep", "no_eligible_backup")
	return bound, boundGroup, nil
}

func selectFailoverCandidate(param *RetryParam, group string, retry int, filters []dto.ChannelFilter, preference *model.ChannelSelectionPreference) (*model.Channel, error) {
	c := param.Ctx
	r := failoverRequestFrom(c)
	if r == nil || r.mode != "enabled" || (retry > 0 && !r.supplement) {
		return model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, preference)
	}
	if _, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned || r.owner.ChannelID != 0 || r.content.kind == "" {
		return model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, preference)
	}
	source := r.failure
	if source == nil {
		source = r.bound
	}
	if !r.supplement && RequestPolicy(c).SessionMode == "strict" && !r.strictException && r.boundID > 0 {
		return model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, preference)
	}
	if preference == nil {
		preference = &model.ChannelSelectionPreference{}
	}
	var trial []*model.Channel
	now := time.Now()
	policy := &model.ChannelSelectionPolicy{Eligible: func(channels []*model.Channel) map[int]bool {
		states := channelFailoverHealthFor(r, c, channels)
		result := make(map[int]bool, len(channels))
		for _, channel := range channels {
			state := states[channel.Id]
			switch {
			case r.failed[channel.Id]:
			case source != nil && source.Id == channel.Id:
				if r.failure == nil && state.trialReady(now) {
					trial = append(trial, channel)
				}
			case source != nil && !r.canMigrateContent(c, source, channel):
			case state.bypassed(now) && (source != nil || r.canMigrateContent(c, nil, channel)):
				if source == nil && state.trialReady(now) {
					trial = append(trial, channel)
				}
			default:
				result[channel.Id] = true
			}
		}
		return result
	}}
	if source != nil && (r.tokenGroup != "auto" || ActiveCustomerContract(c) != nil || group == r.group || group == r.recoveryGroup) {
		priority := source.GetPriority()
		policy.MaxPriority = &priority
	}
	preference.Policy = policy
	channel, err := model.GetSatisfiedChannelWithPreference(group, param.ModelName, 0, filters, preference)
	if err != nil {
		return nil, err
	}
	if channel != nil && r.supplement {
		routingEvent(c, channel.Id, "backup", "eligible_failover_channel")
	}

	// A recovered candidate gets one live trial when a replay-compatible
	// backup exists at this or a lower tier. Without any backup, the trial is
	// simply the request's ordinary attempt on that channel.
	if channel == nil || common.RetryTimes > param.GetRetry() {
		for _, candidate := range trial {
			if channel != nil && (candidate.GetPriority() < channel.GetPriority() || !r.canMigrateContent(c, candidate, channel)) ||
				(policy.MaxPriority != nil && candidate.GetPriority() > *policy.MaxPriority) {
				continue
			}
			scope := channelFailoverScope(candidate, r.modelName, r.operation, -1)
			lease, ok, _ := claimChannelFailoverLease(scope, true)
			if ok {
				r.trialScope, r.trialLease = scope, lease
				preference.SelectedPriority = candidate.GetPriority()
				routingEvent(c, candidate.Id, "trial", "probe_recovery_trial")
				return candidate, nil
			}
		}
	}
	// Auto searches all allowed groups before falling back. A bypassed binding
	// falls back in resolveFailoverBinding, and strict can only use a verified backup.
	if channel != nil || usesAutoFailoverSupplement(param) || source != nil && (r.failure == nil || RequestPolicy(c).SessionMode == "strict") {
		return channel, nil
	}

	// Without an eligible backup, ordinary routing still serves the request,
	// avoiding channels that already failed when another one remains.
	channel, err = selectUntriedFailoverCandidate(param, group, retry, filters, preference)
	if channel == nil && err == nil {
		preference.Policy = nil
		channel, err = model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, preference)
	}
	if channel != nil && err == nil {
		routingEvent(c, channel.Id, "ordinary_route", "no_eligible_backup")
	}
	return channel, err
}

func selectUntriedFailoverCandidate(param *RetryParam, group string, retry int, filters []dto.ChannelFilter, preference *model.ChannelSelectionPreference) (*model.Channel, error) {
	if preference == nil {
		preference = &model.ChannelSelectionPreference{}
	}
	failed := failoverRequestFrom(param.Ctx).failed
	preference.Policy = &model.ChannelSelectionPolicy{Eligible: func(channels []*model.Channel) map[int]bool {
		eligible := make(map[int]bool, len(channels))
		for _, channel := range channels {
			eligible[channel.Id] = !failed[channel.Id]
		}
		return eligible
	}}
	return model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, preference)
}

func (r *channelFailoverRequest) recordSelection(c *gin.Context, channel *model.Channel, slot int) {
	if !channel.ChannelInfo.IsMultiKey {
		slot = -1
	}
	r.channel, r.scope = channel, channelFailoverScope(channel, r.modelName, r.operation, slot)
	r.group = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	if r.group == "" {
		r.group = RequestPolicy(c).SelectedGroup
	}
}

func (r *channelFailoverRequest) beginAttempt() {
	r.http = channelAlertHTTP{}
	r.nonStreamTerminal, r.nonStreamObserved = false, false
	r.responseID, r.responseTurn = "", ""
}

func recordFailoverHTTP(c *gin.Context, response *http.Response, transport bool) {
	if r := failoverRequestFrom(c); r != nil {
		r.http = channelAlertHTTP{transport: transport}
		if response != nil {
			r.http.status = response.StatusCode
		}
	}
}

func PreserveChannelFailoverError(c *gin.Context, selectionErr, previous *types.NewAPIError) *types.NewAPIError {
	if r := failoverRequestFrom(c); r != nil && r.supplement && previous != nil {
		routingEvent(c, r.failure.Id, "stop", "no_eligible_backup")
		return previous
	}
	return selectionErr
}

func channelFailoverRetryGuard(c *gin.Context, err *types.NewAPIError, budget int) (bool, *PolicyDecision) {
	r := failoverRequestFrom(c)
	if r == nil || r.mode != "enabled" || r.channel == nil {
		return false, nil
	}
	if _, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned {
		return false, nil
	}
	if r.owner.ChannelID != 0 {
		return false, &PolicyDecision{Action: "stop", Reason: "original_channel_required", Source: "channel_failover"}
	}
	// Failover takes over only verified content whose failed attempt is on a
	// rollout channel or follows a failover decision; all else retries natively.
	takenOver := r.supplement || r.bound != nil || (r.failure != nil && r.failoverSource(r.failure))
	if r.content.kind == "" || !takenOver {
		return false, nil
	}
	if c.Writer.Written() || c.Request.Context().Err() != nil {
		return false, &PolicyDecision{Action: "stop", Reason: "response_started_or_cancelled", Source: "channel_failover"}
	}
	// A client error from a replayed request is evidence against the migration.
	replayRejected := (r.supplement || r.bound != nil && r.bound.Id != r.channel.Id) &&
		r.http.status >= 400 && r.http.status < 500 && r.http.status != http.StatusTooManyRequests
	switch {
	case replayRejected:
		return false, &PolicyDecision{Action: "stop", Reason: "migration_rejected", Source: "channel_failover"}
	case r.supplement && RequestPolicy(c).SessionMode == "strict":
		// The strict-session exception allows one supplement. Prefer sessions
		// keep the remaining native budget, still excluding failed channels.
		return false, &PolicyDecision{Action: "stop", Reason: "failover_attempt_used", Source: "channel_failover"}
	case types.IsSkipRetryError(err) || operationRetryHardExcluded(err, r.http.status):
		return false, &PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	case budget <= 0:
		return false, &PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	strict := r.strictException && (r.failureHealth.bypassed(time.Now()) || r.trialScope == r.scope)
	if r.http.status >= 300 && !operation_setting.ShouldRetryByStatusCode(r.http.status) {
		return false, &PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
	}
	return strict, nil
}

func ChannelFailoverRetryCommitted(c *gin.Context, decision PolicyDecision) {
	r := failoverRequestFrom(c)
	if r != nil && r.mode == "enabled" && r.failure != nil && r.content.kind != "" && decision.Action == "retry" &&
		(r.bound != nil || r.failoverSource(r.failure)) {
		r.supplement = true
		r.healthDeadline = time.Time{}
	}
}

func operationRetryHardExcluded(err *types.NewAPIError, upstreamStatus int) bool {
	return operation_setting.IsAlwaysSkipRetryStatusCode(upstreamStatus) ||
		operation_setting.IsAlwaysSkipRetryStatusCode(err.StatusCode) ||
		operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode())
}

func channelRelayCompleted(c *gin.Context, stream *defaultAffinityRequest, info *relaycommon.RelayInfo) bool {
	status := c.Writer.Status()
	if status < http.StatusOK || status >= http.StatusMultipleChoices || info.PerformanceBusinessRejection {
		return false
	}
	if !info.IsStream && info.StreamStatus == nil {
		if state := failoverRequestFrom(c); state != nil && state.nonStreamObserved {
			return state.nonStreamTerminal
		}
		return c.Writer.Written()
	}
	terminal := info.StreamStatus
	if terminal == nil || !terminal.IsNormalEnd() || terminal.HasErrors() || terminal.ResponseOutcome() != string(relaycommon.ResponseOutcomeCompleted) {
		return false
	}
	if stream == nil {
		stream = &defaultAffinityRequest{}
	}
	count := stream.chatChoiceCount(c, info)
	return count > 0 && (count == 1 || stream.chatChoiceStream == terminal && len(stream.finishedChatChoices) == count)
}

func ObserveChannelFailoverResult(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) {
	r := failoverRequestFrom(c)
	if r == nil || r.channel == nil {
		return
	}
	r.http.status = channelFailureFallbackStatus(c, r.http.status)
	r.http.transport = r.http.transport || r.http.status == 0 && apiErr != nil && apiErr.GetErrorCode() == types.ErrorCodeDoRequestFailed
	completed := apiErr == nil && channelRelayCompleted(c, affinityStreamTracker(c), info)
	if apiErr != nil {
		if r.failed == nil {
			r.failed = make(map[int]bool)
		}
		r.failed[r.channel.Id] = true
		r.failure = r.channel
		if r.group == "" || ActiveCustomerContract(c) != nil {
			r.group = RequestPolicy(c).SelectedGroup
			if r.group == "" {
				r.group = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
			}
		}
	}
	if r.trialLease != "" && r.trialScope == r.scope {
		result := "failed"
		if completed {
			result = "success"
		}
		if c.Request.Context().Err() != nil && !completed {
			result = "unconfirmed"
		}
		committed, err := CompleteChannelFailoverCheck(r.scope, r.trialLease, result, r.http.status, true)
		r.trialLease = ""
		if err != nil || !committed {
			result = "unrecorded"
		}
		routingEvent(c, r.channel.Id, "trial_result", "recovery_trial_"+result)
	} else if r.inRollout(r.channel.Id) && c.Request.Context().Err() == nil &&
		(r.http.transport || r.http.status >= 300 || r.http.status > 0 && completed) {
		health, err := observeChannelFailoverHealth(r.scope, r.http.status)
		if apiErr != nil && err == nil {
			r.failureHealth = health
		}
		if err != nil {
			routingEvent(c, r.channel.Id, "observe", "health_unknown")
		} else if qualifiesChannelFailure(r.http.status) {
			routingEvent(c, r.channel.Id, "observe", "upstream_unavailable_sample")
		}
	}
	if completed {
		r.recordResponseOwner(c)
	}
	observeChannelFailoverEligibility(c)
}

func observeFailoverEligibility(c *gin.Context, r *channelFailoverRequest) {
	reason := "migration_not_verified"
	if _, pinned, _ := GetChannelConstraints(c).ResolvedPin(); pinned {
		routingEvent(c, r.channel.Id, "observe", "pinned_channel")
		return
	}
	if r.content.kind != "" && r.failoverSource(r.channel) {
		reason = "no_eligible_backup"
		filters := GetChannelConstraints(c).Filters
		group := RequestPolicy(c).SelectedGroup
		if group == "" {
			group = r.tokenGroup
		}
		if ActiveCustomerContract(c) != nil {
			routes, err := customerContractRoutes(c, r.modelName)
			if err != nil {
				routingEvent(c, r.channel.Id, "observe", "contract_route_unavailable")
				return
			}
			filters = append(slices.Clone(filters), dto.ChannelFilter{Kind: dto.FilterContractRoutes, Routes: routes})
		}
		preference := &model.ChannelSelectionPreference{Policy: &model.ChannelSelectionPolicy{
			MaxPriority: common.GetPointer(r.channel.GetPriority()),
			Eligible: func(channels []*model.Channel) map[int]bool {
				states := channelFailoverHealthFor(r, c, channels)
				result := make(map[int]bool, len(channels))
				for _, channel := range channels {
					result[channel.Id] = channel.Id != r.channel.Id && !r.routingBypass(states[channel.Id], time.Now()) &&
						r.canMigrateContent(c, r.channel, channel)
				}
				return result
			},
		}}
		if backup, err := model.GetSatisfiedChannelWithPreference(group, r.modelName, 0, filters, preference); err == nil && backup != nil {
			routingEvent(c, backup.Id, "observe", "eligible_backup")
			return
		}
	}
	routingEvent(c, r.channel.Id, "observe", reason)
}

// A failover supplement uses the same authorized group before considering
// later auto groups. Searching an empty group does not advance the attempt.
func selectAutoFailoverSupplement(param *RetryParam, filters []dto.ChannelFilter) (*model.Channel, string, error) {
	c := param.Ctx
	r := failoverRequestFrom(c)
	groups := GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	if r.group == "" {
		r.group = r.recoveryGroup
	}
	group := r.group
	start := slices.Index(groups, group)
	if start < 0 {
		return nil, group, nil
	}
	groups = groups[start:]
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry) {
		groups = groups[:1]
	}
	selectors := []affinityChannelSelector{selectFailoverCandidate}
	if r.supplement && RequestPolicy(c).SessionMode != "strict" {
		selectors = append(selectors, selectUntriedFailoverCandidate,
			func(param *RetryParam, group string, retry int, filters []dto.ChannelFilter, preference *model.ChannelSelectionPreference) (*model.Channel, error) {
				return model.GetSatisfiedChannelWithPreference(group, param.ModelName, retry, filters, preference)
			})
	}
	for stage, selectCandidate := range selectors {
		for _, group := range groups {
			channel, err := selectChannelWithAffinity(param, group, 0, filters, selectCandidate)
			if err != nil {
				return nil, group, err
			}
			if channel != nil {
				if stage != 0 {
					routingEvent(c, channel.Id, "ordinary_route", "no_eligible_backup")
				}
				common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
				return channel, group, nil
			}
		}
	}
	return nil, group, nil
}

func ChannelRecoveryProbeCompleted(c *gin.Context, info *relaycommon.RelayInfo) bool {
	// Recovery always probes with streaming enabled and requires the same
	// complete terminal evidence as a normal request.
	return info.StreamStatus != nil && channelRelayCompleted(c, nil, info)
}

func observeChannelFailoverEligibility(c *gin.Context) {
	if r := failoverRequestFrom(c); r != nil && r.mode == "observe" && r.channel != nil &&
		r.inRollout(r.channel.Id) && len(c.GetStringSlice("use_channel")) == 1 && !r.observedEligibility {
		r.observedEligibility = true
		observeFailoverEligibility(c, r)
	}
}
