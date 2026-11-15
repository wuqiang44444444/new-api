package controller

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var failoverProbeEndpoints = map[string]constant.EndpointType{
	"messages":  constant.EndpointTypeAnthropic,
	"chat":      constant.EndpointTypeOpenAI,
	"responses": constant.EndpointTypeOpenAIResponse,
}

type channelFailoverHealthRow struct {
	service.ChannelFailoverHealth
	Enabled  bool `json:"enabled"`
	Enforced bool `json:"enforced"`
}

func GetChannelFailoverHealth(c *gin.Context) {
	id := 0
	if raw := c.Param("id"); raw != "" {
		var err error
		id, err = strconv.Atoi(raw)
		if err != nil || id <= 0 {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	rows, err := service.ListChannelFailoverHealth(c.Request.Context(), id)
	if err != nil {
		common.ApiErrorT(c, "Unavailable")
		return
	}
	result := make([]channelFailoverHealthRow, 0, len(rows))
	channels := make(map[int]*model.Channel)
	for _, row := range rows {
		channel, loaded := channels[row.Scope.ChannelID]
		if !loaded {
			channel, _ = model.GetChannelById(row.Scope.ChannelID, true)
			channels[row.Scope.ChannelID] = channel
		}
		if channel == nil || model.ChannelAlertRevision(channel) != row.Scope.Revision {
			continue
		}
		row.Lease = ""
		row.Scope.Revision = ""
		if row.State == "cooldown" && !row.NextCheck.After(time.Now()) {
			row.State = "pending"
		}
		result = append(result, channelFailoverHealthRow{
			ChannelFailoverHealth: row,
			Enabled:               channel.Status == common.ChannelStatusEnabled,
			Enforced:              model.CurrentRequestPolicy().Affinity.FailoverMode == "enabled" && service.ChannelFailoverScopeConfigured(row.Scope) && row.Scope.Slot == -1,
		})
	}
	common.ApiSuccess(c, result)
}

func UpdateChannelFailoverHealth(c *gin.Context) {
	var request struct {
		Model     string `json:"model"`
		Operation string `json:"operation"`
		Action    string `json:"action"`
		Minutes   int    `json:"minutes"`
		Reason    string `json:"reason"`
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 || common.DecodeJson(c.Request.Body, &request) != nil || failoverProbeEndpoints[request.Operation] == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil || !service.ChannelFailoverConfigured() || channel.Status != common.ChannelStatusEnabled ||
		channel.ChannelInfo.IsMultiKey || !slices.Contains(channel.GetModels(), request.Model) {
		common.ApiErrorT(c, "Unavailable")
		return
	}
	scope := service.ChannelFailoverScope{
		ChannelID: id, Revision: model.ChannelAlertRevision(channel), Model: request.Model, Operation: request.Operation, Slot: -1,
	}
	if !service.ChannelFailoverScopeConfigured(scope) {
		common.ApiErrorT(c, "Unavailable")
		return
	}
	switch request.Action {
	case "bypass", "clear_bypass":
		duration := time.Duration(0)
		reason := strings.TrimSpace(request.Reason)
		if request.Action == "bypass" {
			if request.Minutes < 1 || request.Minutes > 1440 || reason == "" || len(reason) > 240 {
				common.ApiErrorT(c, "Invalid parameters")
				return
			}
			duration = time.Duration(request.Minutes) * time.Minute
		}
		err = service.SetChannelFailoverBypass(scope, duration, reason)
	case "probe", "permit_trial":
		var userID int
		userID, err = resolveChannelTestUserID(c)
		if err == nil {
			err = runFailoverProbe(c.Request.Context(), channel, scope, userID, request.Action == "permit_trial")
		}
	default:
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err != nil {
		common.ApiErrorT(c, "Unavailable")
		return
	}
	common.ApiSuccess(c, nil)
}

func runFailoverProbe(ctx context.Context, channel *model.Channel, scope service.ChannelFailoverScope, userID int, permitTrial bool) error {
	lease, acquired, err := service.ClaimChannelFailoverProbe(scope)
	if err != nil {
		return err
	}
	if !acquired {
		return common.NewMessage("Unavailable")
	}
	if permitTrial {
		return completeFailoverProbe(scope, lease, "manual_trial", 0)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, routingRecoveryProbeContextKey{}, true)
	if !service.ChannelFailoverSupportsProbe(channel, scope.Operation) ||
		validateChannelAutoCheckSettings(channel) != nil || channelAutoCheckScopeForChannel(channel) != "generation_probe" {
		return completeFailoverProbe(scope, lease, "unconfirmed", 0)
	}
	result := testChannel(ctx, channel, userID, scope.Model, string(failoverProbeEndpoints[scope.Operation]), true)
	return completeFailoverProbe(scope, lease, result.recoveryEvidence, result.upstreamStatus)
}

// An operator must not see success when the lease expired or the write failed.
func completeFailoverProbe(scope service.ChannelFailoverScope, lease, result string, status int) error {
	committed, err := service.CompleteChannelFailoverCheck(scope, lease, result, status, false)
	if err == nil && !committed {
		err = common.NewMessage("Unavailable")
	}
	return err
}

type routingRecoveryProbeContextKey struct{}

func isRoutingRecoveryProbe(ctx context.Context) bool {
	probe, _ := ctx.Value(routingRecoveryProbeContextKey{}).(bool)
	return probe
}

// routingRecoveryEvidence counts only qualifying upstream statuses and a
// transport failure observed before any response as failed recovery evidence.
func routingRecoveryEvidence(ctx context.Context, result testResult) string {
	if !isRoutingRecoveryProbe(ctx) || result.recoveryEvidence != "" {
		return result.recoveryEvidence
	}
	if ctx.Err() == nil && (service.IsChannelFailoverProbeFailure(result.upstreamStatus) ||
		result.upstreamStatus == 0 && result.upstreamAttempted && !result.upstreamResponded &&
			result.newAPIError != nil && result.newAPIError.GetErrorCode() == types.ErrorCodeDoRequestFailed) {
		return "failed"
	}
	return "unconfirmed"
}

func routingRecoveryProbeOutcome(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) string {
	if response != nil && response.StatusCode == http.StatusOK && service.ChannelRecoveryProbeCompleted(c, info) {
		return "success"
	}
	return "unconfirmed"
}

func clearChannelAffinityLayer(c *gin.Context) bool {
	layer := c.Query("layer")
	if layer == "" {
		return false
	}
	deleted, err := service.ClearChannelAffinityLayer(layer)
	if err != nil {
		common.ApiErrorT(c, "Operation failed")
		return true
	}
	common.ApiSuccess(c, gin.H{"deleted": deleted})
	return true
}

func runDueChannelFailoverProbes(ctx context.Context, testUserID int) error {
	if !service.ChannelFailoverConfigured() {
		return nil
	}
	rows, err := service.ListChannelFailoverHealth(ctx, 0)
	if err != nil {
		return err
	}
	remaining := 4
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		if remaining == 0 || ctx.Err() != nil {
			break
		}
		if (row.State != "cooldown" && row.State != "pending") || row.NextCheck.After(time.Now()) ||
			row.LeaseUntil.After(time.Now()) || row.ManualUntil.After(time.Now()) ||
			row.Scope.Slot != -1 || failoverProbeEndpoints[row.Scope.Operation] == "" ||
			!service.ChannelFailoverScopeConfigured(row.Scope) {
			continue
		}
		channel, err := model.GetChannelById(row.Scope.ChannelID, true)
		if err != nil || channel.Status != common.ChannelStatusEnabled ||
			channel.ChannelInfo.IsMultiKey || model.ChannelAlertRevision(channel) != row.Scope.Revision ||
			!slices.Contains(channel.GetModels(), row.Scope.Model) {
			continue
		}
		remaining--
		_ = runFailoverProbe(ctx, channel, row.Scope, testUserID, false)
	}
	return ctx.Err()
}

type channelFailoverProbeHandler struct{}

func (channelFailoverProbeHandler) Type() string { return "channel_failover_probe" }
func (channelFailoverProbeHandler) Enabled() bool {
	return model.CurrentRequestPolicy().Affinity.FailoverMode == "enabled"
}
func (channelFailoverProbeHandler) Interval() time.Duration { return time.Minute }
func (channelFailoverProbeHandler) NewPayload() any         { return nil }

func (channelFailoverProbeHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	userID, err := resolveChannelTestUserID(nil)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	if err := runDueChannelFailoverProbes(ctx, userID); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, nil, nil)
}
