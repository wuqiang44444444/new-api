package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	arkSessionMaxRounds       = 8
	arkSessionMaxTTLSeconds   = 1800
	arkSessionMaxInputTokens  = 8192
	arkSessionMaxOutputTokens = 1024
	arkSessionContextOverhead = 256
)

func handleArkSessionWrite(c *gin.Context, pilot *service.ArkPrefixCachePilot, request arkCacheRequest) {
	ctx := c.Request.Context()
	now := common.GetTimestamp()
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 191 {
		arkCacheError(c, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	if request.MaxOutput == nil {
		request.MaxOutput = common.GetPointer(uint(512))
	}
	if *request.MaxOutput > arkSessionMaxOutputTokens || len(request.Input) > arkSessionMaxInputTokens {
		arkCacheError(c, http.StatusBadRequest, "cache_session_input_limit")
		return
	}
	normalized, err := common.Marshal(request)
	if err != nil {
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_request")
		return
	}
	row := model.ArkPrefixCache{
		ID: model.ArkSessionCacheIDPrefix + common.GetUUID(), UserID: pilot.UserID, TokenID: pilot.TokenID,
		Model: request.Model, GroupName: common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		Username: c.GetString("username"), TokenName: c.GetString("token_name"), ChannelID: pilot.ChannelID,
		Endpoint: pilot.Endpoint, State: model.ArkCacheSending, SessionRound: 1,
		KeyDigest: model.ArkSessionCacheKeyDigest(key), RequestDigest: service.ArkCacheDigest(string(normalized)),
	}
	prior, err := model.FindArkPrefixCacheClaim(ctx, row.UserID, row.TokenID, row.KeyDigest)
	if err == nil {
		replayArkSession(c, row.RequestDigest, prior)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		arkCacheStateError(c, err)
		return
	}
	if pilot.Mode != "observe" {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_session_observation_required")
		return
	}
	if common.BatchUpdateEnabled {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_session_requires_committed_balances")
		return
	}
	if pilot.LocalSessionStorage {
		row.MeterVersion = model.ArkSessionLocalMeterVersion
	}
	var priorTokens int
	if request.PreviousResponse == "" {
		if request.ExpireAt == nil || *request.ExpireAt <= now || *request.ExpireAt > now+min(pilot.MaxTTLSeconds, arkSessionMaxTTLSeconds) {
			arkCacheError(c, http.StatusBadRequest, "invalid_cache_expiry")
			return
		}
		row.ExpiresAt = *request.ExpireAt
	} else {
		parent, err := model.FindArkPrefixCache(ctx, row.UserID, row.TokenID, request.PreviousResponse)
		if err != nil || parent.SessionRound == 0 || parent.InferenceSettledAt == 0 ||
			parent.State != model.ArkCacheActive || parent.ExpiresAt <= now || parent.Model != row.Model || parent.MeterVersion != row.MeterVersion {
			arkCacheError(c, http.StatusNotFound, "cache_not_found")
			return
		}
		if parent.SessionRound >= arkSessionMaxRounds {
			arkCacheError(c, http.StatusTooManyRequests, "cache_session_round_limit")
			return
		}
		priorTokens = parent.InputTokens + parent.OutputTokens
		row.ParentID, row.ChannelID, row.Endpoint, row.Credential = parent.ID, parent.ChannelID, parent.Endpoint, parent.Credential
		row.ExpiresAt, row.SessionRound = parent.ExpiresAt, parent.SessionRound+1
		request.PreviousResponse = parent.ProviderID
		request.ExpireAt = &row.ExpiresAt
	}
	// Count serialized input bytes conservatively, including message wrappers.
	// The Provider's actual tokenizer remains authoritative on the response.
	if !arkSessionContextWithinLimit(priorTokens, len(request.Input)) {
		arkCacheError(c, http.StatusBadRequest, "cache_session_context_limit")
		return
	}
	channel, credential := service.ArkCacheChannel(ctx, row.ChannelID, row.Credential)
	if channel == nil || channel.Status != common.ChannelStatusEnabled {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
		return
	}
	if !authorizeArkCacheWrite(c, pilot, channel, &row) {
		return
	}
	row.Credential = service.ArkCacheDigest(credential)
	row.RequestedUntil = row.ExpiresAt
	common.SetContextKey(c, constant.ContextKeyOriginalModel, row.Model)
	billing := relaycommon.GenRelayInfoResponses(c, &dto.OpenAIResponsesRequest{Model: row.Model, Input: request.Input, MaxOutputTokens: request.MaxOutput})
	billing.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: row.ChannelID, ChannelType: constant.ChannelTypeVolcEngine,
		ApiType: constant.APITypeVolcEngine, UpstreamModelName: service.ArkPrefixCacheModelVersion, IsModelMapped: true}
	switch common.NormalizeBillingPreference(billing.UserSetting.BillingPreference) {
	case "subscription_only":
		arkCacheError(c, http.StatusForbidden, "cache_session_requires_wallet")
		return
	case "subscription_first":
		active, err := model.HasActiveUserSubscription(row.UserID)
		if err != nil {
			arkCacheStateError(c, err)
			return
		}
		if active {
			arkCacheError(c, http.StatusForbidden, "cache_session_requires_wallet")
			return
		}
	}
	billing.SetEstimatePromptTokens(arkSessionMaxInputTokens)
	if _, err := helper.ModelPriceHelper(c, billing, arkSessionMaxInputTokens, &types.TokenCountMeta{MaxTokens: int(*request.MaxOutput)}); err != nil {
		arkCacheError(c, http.StatusBadRequest, "cache_session_price_unavailable")
		return
	}
	row.InferenceHeldQuota, err = service.ArkSessionInferenceHold(billing, arkSessionMaxInputTokens, int(*request.MaxOutput))
	if err != nil {
		arkCacheError(c, http.StatusBadRequest, "cache_session_price_unavailable")
		return
	}
	row.SentAt = common.GetTimestamp()
	if row.ExpiresAt <= row.SentAt {
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_expiry")
		return
	}
	row.InferenceNextAt = row.SentAt + model.ArkCacheUnknownAfterSeconds
	if pilot.LocalSessionStorage {
		if err := row.ReserveArkSessionLocalStorage(pilot.Price, arkSessionMaxInputTokens+int(*request.MaxOutput)); err != nil {
			arkCacheError(c, http.StatusServiceUnavailable, "cache_storage_metering_paused")
			return
		}
	}
	priorClaim, err := model.ClaimArkPrefixCache(ctx, &row, pilot.MaxActive, pilot.MaxHourlyWrites, pilot.MaxHeld, row.SentAt)
	if err != nil {
		arkCacheStateError(c, err)
		return
	}
	if priorClaim != nil {
		replayArkSession(c, row.RequestDigest, *priorClaim)
		return
	}
	request.Model = row.Endpoint
	request.Metadata = map[string]string{"gateway_attempt": row.ID}
	reply, status, upstreamErr := service.SendArkCacheRequest(ctx, credential, http.MethodPost, "", &request)
	row.State = model.ArkCacheUnknown
	if upstreamErr == nil && reply.Object == "response" && service.ValidArkCacheProviderID(reply.ID) &&
		(reply.Model == row.Endpoint || reply.Model == service.ArkPrefixCacheModelVersion) &&
		reply.PreviousResponseID == request.PreviousResponse {
		row.ProviderID = reply.ID
		if reply.Status == "completed" && reply.Store && reply.Caching.Type == "enabled" &&
			reply.CreatedAt >= row.SentAt-60 && reply.CreatedAt <= row.SentAt+60 &&
			reply.ExpireAt > reply.CreatedAt && reply.ExpireAt <= row.RequestedUntil {
			row.CreatedAt, row.ExpiresAt = reply.CreatedAt, reply.ExpireAt
			usage := arkSessionInferenceUsage(reply.Usage, int(*request.MaxOutput))
			if usage != nil {
				row.InputTokens, row.OutputTokens, row.ReadTokens = usage.PromptTokens, usage.CompletionTokens, usage.PromptTokensDetails.CachedTokens
				row.InferenceQuota, row.InferenceLog, err = service.ArkSessionInferenceFacts(c, billing, usage)
				if err == nil && row.InferenceQuota >= 0 && row.InferenceQuota <= row.InferenceHeldQuota {
					row.State = model.ArkCacheInferencePending
				}
			}
		}
	} else if service.ArkCacheCreateRejected(reply, status) {
		row.State, row.RejectedStatus = model.ArkCacheRejected, status
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := persistArkCacheReceipt(commitCtx, row, model.CompleteArkPrefixCache); err != nil || row.State == model.ArkCacheUnknown {
		arkCacheError(c, http.StatusConflict, "cache_create_outcome_unknown")
		return
	}
	settleErr := model.SettleArkSessionInference(commitCtx, row.ID, common.GetTimestamp())
	if row.State == model.ArkCacheRejected {
		arkCacheError(c, status, "cache_provider_rejected")
		return
	}
	if settleErr != nil {
		arkCacheError(c, http.StatusConflict, "cache_billing_pending")
		return
	}
	if err := model.DeliverArkSessionLog(commitCtx, row.ID); err != nil {
		common.SysError("Ark Session consumption log awaiting delivery")
	}
	c.JSON(http.StatusOK, arkCachePublicResponse(row, arkCachePublicOutput(reply.Output)))
}

func arkSessionContextWithinLimit(historyTokens, inputBytes int) bool {
	return historyTokens >= 0 && inputBytes >= 0 &&
		historyTokens <= arkSessionMaxInputTokens-arkSessionContextOverhead &&
		inputBytes <= arkSessionMaxInputTokens-arkSessionContextOverhead-historyTokens
}

// persistArkCacheReceipt retries the local write only. Each attempt has its own
// deadline so a brief outage can recover while the trusted reply is in memory.
func persistArkCacheReceipt(ctx context.Context, row model.ArkPrefixCache, commit func(context.Context, model.ArkPrefixCache) error) error {
	var err error
	for _, delay := range []time.Duration{0, 100 * time.Millisecond, 250 * time.Millisecond} {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, time.Second)
		err = commit(attemptCtx, row)
		cancel()
		if err == nil || errors.Is(err, model.ErrArkCacheBusy) {
			return err
		}
	}
	return err
}

func arkSessionInferenceUsage(raw common.RawMessage, outputLimit int) *dto.Usage {
	var meter struct {
		Input   *int `json:"input_tokens"`
		Output  *int `json:"output_tokens"`
		Details struct {
			Cached *int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	}
	if common.Unmarshal(raw, &meter) != nil || meter.Input == nil || meter.Output == nil || meter.Details.Cached == nil {
		return nil
	}
	input, output, cached := *meter.Input, *meter.Output, *meter.Details.Cached
	if input <= 0 || input > arkSessionMaxInputTokens || output < 0 || output > outputLimit || cached < 0 || cached > input {
		return nil
	}
	return &dto.Usage{PromptTokens: input, InputTokens: input, CompletionTokens: output, TotalTokens: input + output,
		PromptTokensDetails: dto.InputTokenDetails{CachedTokens: cached}}
}

func replayArkSession(c *gin.Context, digest string, row model.ArkPrefixCache) {
	if row.RequestDigest != digest {
		arkCacheError(c, http.StatusConflict, "idempotency_conflict")
		return
	}
	if row.State == model.ArkCacheInferencePending {
		arkCacheError(c, http.StatusConflict, "cache_billing_pending")
		return
	}
	if row.InferenceSettledAt == 0 {
		switch row.State {
		case model.ArkCacheSending, model.ArkCacheRejected:
			arkCacheReplay(c, digest, row)
		default:
			arkCacheError(c, http.StatusConflict, "cache_create_outcome_unknown")
		}
		return
	}
	if row.State != model.ArkCacheActive || row.ExpiresAt <= common.GetTimestamp() {
		arkCacheError(c, http.StatusGone, "cache_session_response_unavailable")
		return
	}
	ctx := c.Request.Context()
	credential := service.ArkCacheResourceCredential(row)
	if credential == "" {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
		return
	}
	reply, status, err := service.SendArkCacheRequest(ctx, credential, http.MethodGet, row.ProviderID, nil)
	if service.IsArkCacheMissing(reply, status) {
		arkCacheError(c, http.StatusGone, "cache_session_response_unavailable")
		return
	}
	if err != nil || reply.ID != row.ProviderID || reply.Status != "completed" || reply.Object != "response" ||
		(reply.Model != row.Endpoint && reply.Model != service.ArkPrefixCacheModelVersion) {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
		return
	}
	c.JSON(http.StatusOK, arkCachePublicResponse(row, arkCachePublicOutput(reply.Output)))
}

func arkCachePublicOutput(items []service.ArkCacheOutput) []service.ArkCacheOutput {
	output := make([]service.ArkCacheOutput, 0, len(items))
	for _, item := range items {
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}
		content := item.Content[:0]
		for _, part := range item.Content {
			if part.Type == "output_text" || part.Type == "refusal" {
				content = append(content, part)
			}
		}
		item.Content = content
		output = append(output, item)
	}
	return output
}
