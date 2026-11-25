package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type arkCacheRequest struct {
	Model            string            `json:"model"`
	Input            common.RawMessage `json:"input"`
	Store            *bool             `json:"store,omitempty"`
	Stream           *bool             `json:"stream,omitempty"`
	ExpireAt         *int64            `json:"expire_at,omitempty"`
	PreviousResponse string            `json:"previous_response_id,omitempty"`
	MaxOutput        *uint             `json:"max_output_tokens,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	Caching          struct {
		Type   string `json:"type"`
		Prefix *bool  `json:"prefix,omitempty"`
	} `json:"caching"`
	Thinking struct {
		Type string `json:"type"`
	} `json:"thinking"`
}

// ArkPrefixCacheResponses shares the existing POST registration. The ordinary
// path performs no cache DB queries, even when the pilot is enabled.
func ArkPrefixCacheResponses(c *gin.Context) {
	arkPrefixCacheResponses(c, service.ArkCachePilot())
}

func arkPrefixCacheResponses(c *gin.Context, pilot *service.ArkPrefixCachePilot) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return
	}
	if pilot == nil {
		fields, err := common.ReadBodyJSONFields(storage, "previous_response_id")
		if err == nil && len(fields) == 1 && model.IsArkManagedCacheID(fields[0].String()) {
			c.Abort()
			arkCacheError(c, http.StatusNotFound, "cache_not_found")
		}
		return
	}
	fields, err := common.ReadBodyJSONFields(storage, "model", "previous_response_id", "caching.type", "caching.prefix")
	if err != nil || len(fields) != 4 {
		return
	}
	ownApp := c.GetInt("id") == pilot.UserID && c.GetInt("token_id") == pilot.TokenID
	priorID := fields[1].String()
	managed := model.IsArkManagedCacheID(priorID)
	selected := ownApp && fields[0].String() == pilot.Model && fields[2].String() == "enabled" && (priorID == "" || fields[3].Bool())
	if !managed && !selected {
		return
	}
	c.Abort()
	if !ownApp || fields[0].String() != pilot.Model {
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	common.SetContextKey(c, constant.ContextKeyOriginalModel, pilot.Model)
	if storage.Size() > int64(6*pilot.MaxInputBytes+4096) {
		arkCacheError(c, http.StatusRequestEntityTooLarge, "cache_input_too_large")
		return
	}
	var request arkCacheRequest
	var raw map[string]common.RawMessage
	if common.UnmarshalBodyReusable(c, &raw) != nil || common.UnmarshalBodyReusable(c, &request) != nil || request.Model != pilot.Model {
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_request")
		return
	}
	for field := range raw {
		switch field {
		case "model", "input", "store", "stream", "expire_at", "previous_response_id", "caching", "thinking", "max_output_tokens":
		default:
			arkCacheError(c, http.StatusBadRequest, "unsupported_cache_parameter")
			return
		}
	}
	if !arkCacheOnlyFields(raw["caching"], "type", "prefix") ||
		(raw["thinking"] != nil && !arkCacheOnlyFields(raw["thinking"], "type")) {
		arkCacheError(c, http.StatusBadRequest, "unsupported_cache_parameter")
		return
	}
	textInput, validInput := arkCacheTextInput(request.Input, pilot.MaxInputBytes)
	prefix := request.Caching.Prefix != nil && *request.Caching.Prefix
	create := prefix && request.PreviousResponse == ""
	session := !prefix && (request.PreviousResponse == "" || strings.HasPrefix(request.PreviousResponse, model.ArkSessionCacheIDPrefix))
	if request.PreviousResponse != priorID || (!create && !session && !managed) || (prefix && !create) ||
		request.Caching.Type != "enabled" || request.Store == nil || !*request.Store || (request.Stream != nil && *request.Stream) || !validInput ||
		(request.Thinking.Type != "" && request.Thinking.Type != "disabled") ||
		(request.MaxOutput != nil && (*request.MaxOutput < 16 || *request.MaxOutput > 4096)) ||
		(create && (request.PreviousResponse != "" || request.MaxOutput != nil)) ||
		(managed && (prefix || request.ExpireAt != nil)) {
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_request")
		return
	}
	request.Input = textInput
	request.Thinking.Type = "disabled"
	if !service.ContractTokenModelAllowed(c, request.Model) {
		arkCacheError(c, http.StatusForbidden, "cache_model_forbidden")
		return
	}
	if service.ActiveCustomerContract(c) == nil && common.GetContextKeyString(c, constant.ContextKeyUsingGroup) == "auto" {
		arkCacheError(c, http.StatusForbidden, "cache_pilot_requires_fixed_group")
		return
	}
	if session {
		handleArkSessionWrite(c, pilot, request)
		return
	}
	handleArkCacheWrite(c, pilot, request, create)
}

func arkCacheOnlyFields(raw common.RawMessage, allowed ...string) bool {
	var fields map[string]common.RawMessage
	if common.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
		return false
	}
	for _, name := range allowed {
		delete(fields, name)
	}
	return len(fields) == 0
}

func arkCacheTextInput(raw common.RawMessage, limit int) (common.RawMessage, bool) {
	var text string
	if common.Unmarshal(raw, &text) == nil {
		if strings.TrimSpace(text) == "" || len(text) > limit {
			return nil, false
		}
		encoded, err := common.Marshal(text)
		return encoded, err == nil
	}
	var messages []common.RawMessage
	if common.Unmarshal(raw, &messages) != nil || len(messages) == 0 {
		return nil, false
	}
	type messageInput struct {
		Type    string            `json:"type,omitempty"`
		Role    string            `json:"role"`
		Content common.RawMessage `json:"content"`
	}
	type textPart struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	inputs := make([]messageInput, 0, len(messages))
	size := 0
	for _, item := range messages {
		var message messageInput
		if !arkCacheOnlyFields(item, "type", "role", "content") || common.Unmarshal(item, &message) != nil ||
			(message.Type != "" && message.Type != "message") {
			return nil, false
		}
		switch message.Role {
		case "system", "developer", "user", "assistant":
		default:
			return nil, false
		}
		if common.Unmarshal(message.Content, &text) == nil {
			if text == "" {
				return nil, false
			}
			size += len(text)
			message.Content, _ = common.Marshal(text)
			inputs = append(inputs, message)
			continue
		}
		var parts []common.RawMessage
		if common.Unmarshal(message.Content, &parts) != nil || len(parts) == 0 {
			return nil, false
		}
		textParts := make([]textPart, 0, len(parts))
		for _, rawPart := range parts {
			var part textPart
			if !arkCacheOnlyFields(rawPart, "type", "text") || common.Unmarshal(rawPart, &part) != nil || part.Type != "input_text" || part.Text == "" {
				return nil, false
			}
			size += len(part.Text)
			textParts = append(textParts, part)
		}
		message.Content, _ = common.Marshal(textParts)
		inputs = append(inputs, message)
	}
	if size == 0 || size > limit {
		return nil, false
	}
	encoded, err := common.Marshal(inputs)
	return encoded, err == nil
}

func handleArkCacheWrite(c *gin.Context, pilot *service.ArkPrefixCachePilot, request arkCacheRequest, create bool) {
	ctx := c.Request.Context()
	now := common.GetTimestamp()
	row := model.ArkPrefixCache{
		ID: model.ArkPrefixCacheIDPrefix + common.GetUUID(), UserID: pilot.UserID, TokenID: pilot.TokenID,
		Model: request.Model, GroupName: common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		Username: c.GetString("username"), TokenName: c.GetString("token_name"), ChannelID: pilot.ChannelID,
		Endpoint: pilot.Endpoint, State: model.ArkCacheSending,
	}
	if create {
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if key == "" || len(key) > 191 {
			arkCacheError(c, http.StatusBadRequest, "invalid_idempotency_key")
			return
		}
		row.KeyDigest = model.ArkPrefixCacheKeyDigest(key)
		normalized, _ := common.Marshal(request)
		row.RequestDigest = service.ArkCacheDigest(string(normalized))
		prior, err := model.FindArkPrefixCacheClaim(ctx, row.UserID, row.TokenID, row.KeyDigest)
		if err == nil {
			arkCacheReplay(c, row.RequestDigest, prior)
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
			return
		}
		if request.ExpireAt == nil || *request.ExpireAt <= now || *request.ExpireAt > now+pilot.MaxTTLSeconds {
			arkCacheError(c, http.StatusBadRequest, "invalid_cache_expiry")
			return
		}
		row.ExpiresAt = *request.ExpireAt
	} else {
		parent, err := model.FindArkPrefixCache(ctx, row.UserID, row.TokenID, request.PreviousResponse)
		if err != nil || parent.ParentID != "" || parent.State != model.ArkCacheActive || parent.ExpiresAt <= now || parent.Model != row.Model {
			arkCacheError(c, http.StatusNotFound, "cache_not_found")
			return
		}
		row.ParentID, row.ChannelID, row.Endpoint, row.Credential = parent.ID, parent.ChannelID, parent.Endpoint, parent.Credential
		row.ExpiresAt = parent.ExpiresAt
		request.PreviousResponse = parent.ProviderID
		request.ExpireAt = &row.ExpiresAt
		row.KeyDigest = service.ArkCacheDigest(row.ID)
	}
	if pilot.Mode == "disabled" {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_disabled")
		return
	}
	if pilot.Mode != "observe" {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_storage_metering_paused")
		return
	}
	channel, key := service.ArkCacheChannel(ctx, row.ChannelID, row.Credential)
	if channel == nil || channel.Status != common.ChannelStatusEnabled {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
		return
	}
	if !authorizeArkCacheWrite(c, pilot, channel, &row) {
		return
	}
	row.Credential = service.ArkCacheDigest(key)
	row.RequestedUntil = row.ExpiresAt
	if request.MaxOutput == nil && !create {
		request.MaxOutput = common.GetPointer(uint(1024))
	}
	common.SetContextKey(c, constant.ContextKeyOriginalModel, row.Model)
	billing := relaycommon.GenRelayInfoResponses(c, &dto.OpenAIResponsesRequest{Model: row.Model, Input: request.Input, MaxOutputTokens: request.MaxOutput})
	billing.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: row.ChannelID, ChannelType: constant.ChannelTypeVolcEngine,
		ApiType: constant.APITypeVolcEngine, UpstreamModelName: service.ArkPrefixCacheModelVersion, IsModelMapped: true}
	if apiErr := relay.PrepareRequestBilling(c, billing); apiErr != nil {
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}
	defer func() {
		if billing.Billing != nil {
			billing.Billing.Refund(c)
		}
	}()
	row.SentAt = common.GetTimestamp()
	if row.ExpiresAt <= row.SentAt {
		arkCacheError(c, http.StatusBadRequest, "invalid_cache_expiry")
		return
	}
	prior, err := model.ClaimArkPrefixCache(ctx, &row, pilot.MaxActive, pilot.MaxHourlyWrites, pilot.MaxHeld, row.SentAt)
	if err != nil {
		arkCacheStateError(c, err)
		return
	}
	if prior != nil {
		arkCacheReplay(c, row.RequestDigest, *prior)
		return
	}
	request.Model = row.Endpoint
	response, status, err := service.SendArkCacheRequest(ctx, key, http.MethodPost, "", &request)
	row.State = model.ArkCacheUnknown
	trustedResponse := err == nil && response.Object == "response" && service.ValidArkCacheProviderID(response.ID) &&
		(response.Model == row.Endpoint || response.Model == service.ArkPrefixCacheModelVersion)
	if trustedResponse {
		row.ProviderID = response.ID
	}
	if trustedResponse && response.Status == "completed" && response.CreatedAt >= row.SentAt-3600 && response.CreatedAt > 0 &&
		(response.ExpireAt == 0 && !create && row.MeterVersion == "" || (response.ExpireAt > response.CreatedAt && response.ExpireAt <= row.RequestedUntil)) {
		row.State, row.CreatedAt = model.ArkCacheActive, response.CreatedAt
		row.NextHour = row.CreatedAt - row.CreatedAt%3600
		if response.ExpireAt > response.CreatedAt {
			row.ExpiresAt = response.ExpireAt
		}
		var reported *dto.Usage
		if common.Unmarshal(response.Usage, &reported) == nil && reported != nil {
			var usage dto.Usage
			service.ApplyResponsesUsage(&usage, reported)
			row.InputTokens, row.OutputTokens = usage.PromptTokens, usage.CompletionTokens
			row.ReadTokens = usage.PromptTokensDetails.CachedTokens
			// A successful provider call keeps the native inference meter.
			relay.ConsumeResponsesQuota(c, billing, &usage)
		} else {
			relay.ConsumeResponsesQuota(c, billing, nil)
		}
	} else if service.ArkCacheCreateRejected(response, status) {
		row.State, row.RejectedStatus = model.ArkCacheRejected, status
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := persistArkCacheReceipt(commitCtx, row, model.CompleteArkPrefixCache); err != nil || row.State == model.ArkCacheUnknown {
		arkCacheError(c, http.StatusConflict, "cache_create_outcome_unknown")
		return
	}
	if row.State == model.ArkCacheRejected {
		arkCacheError(c, status, "cache_provider_rejected")
		return
	}
	if create {
		arkCacheReplay(c, row.RequestDigest, row)
		return
	}
	c.JSON(http.StatusOK, arkCachePublicResponse(row, arkCachePublicOutput(response.Output)))
}

func authorizeArkCacheWrite(c *gin.Context, pilot *service.ArkPrefixCachePilot, channel *model.Channel, row *model.ArkPrefixCache) bool {
	if err := service.AuthorizeArkCacheContract(c, pilot, channel, row.Model); err != nil {
		if errors.Is(err, service.ErrCustomerContractScope) {
			arkCacheError(c, http.StatusForbidden, "model_not_found")
		} else {
			arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
		}
		return false
	}
	row.GroupName = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	return true
}

func arkCacheRejected(status int) bool {
	switch status {
	case 400, 401, 403, 404, 422, 429:
		return true
	default:
		return false
	}
}

func arkCacheError(c *gin.Context, status int, code string) {
	if c.Request != nil {
		clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{Stage: "ark_cache", Reason: code, PublicCode: code})
	}
	c.JSON(status, gin.H{"error": gin.H{"message": strings.ReplaceAll(code, "_", " "), "code": code, "type": "ark_prefix_cache_error"}})
}

func arkCacheStateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrArkCacheConflict):
		arkCacheError(c, http.StatusConflict, "idempotency_conflict")
	case errors.Is(err, model.ErrArkCacheLimit):
		arkCacheError(c, http.StatusTooManyRequests, "cache_admission_limit")
	case errors.Is(err, model.ErrArkCacheFunds):
		arkCacheError(c, http.StatusForbidden, "insufficient_user_quota")
	case errors.Is(err, model.ErrArkCacheBusy):
		arkCacheError(c, http.StatusConflict, "cache_operation_pending")
	case errors.Is(err, model.ErrArkCacheMeter):
		arkCacheError(c, http.StatusServiceUnavailable, "cache_storage_metering_paused")
	default:
		arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
	}
}

func arkCacheReplay(c *gin.Context, digest string, row model.ArkPrefixCache) {
	if row.RequestDigest != digest {
		arkCacheError(c, http.StatusConflict, "idempotency_conflict")
		return
	}
	switch row.State {
	case model.ArkCacheActive, model.ArkCacheDeleting, model.ArkCacheDeleted:
		if row.CreatedAt == 0 || row.ProviderID == "" {
			arkCacheError(c, http.StatusConflict, "cache_create_outcome_unknown")
			return
		}
		c.JSON(http.StatusOK, arkCachePublicResponse(row, []service.ArkCacheOutput{}))
	case model.ArkCacheSending:
		if common.GetTimestamp()-row.SentAt > 65 {
			arkCacheError(c, http.StatusConflict, "cache_create_outcome_unknown")
		} else {
			arkCacheError(c, http.StatusConflict, "cache_create_in_progress")
		}
	case model.ArkCacheRejected:
		status := row.RejectedStatus
		if !arkCacheRejected(status) {
			status = http.StatusBadRequest
		}
		arkCacheError(c, status, "cache_provider_rejected")
	default:
		arkCacheError(c, http.StatusConflict, "cache_create_outcome_unknown")
	}
}

func arkCachePublicResponse(row model.ArkPrefixCache, output []service.ArkCacheOutput) gin.H {
	return gin.H{"id": row.ID, "object": "response", "model": row.Model, "status": "completed", "output": output,
		"created_at": row.CreatedAt, "expire_at": row.ExpiresAt, "store": true,
		"caching": gin.H{"type": "enabled", "prefix": row.ParentID == "" && row.SessionRound == 0},
		"usage": gin.H{"input_tokens": row.InputTokens, "output_tokens": row.OutputTokens,
			"input_tokens_details": gin.H{"cached_tokens": row.ReadTokens}, "total_tokens": row.InputTokens + row.OutputTokens}}
}

func DeleteArkPrefixCache(c *gin.Context) {
	id := c.Param("response_id")
	if !model.IsArkManagedCacheID(id) || len(id) > 64 {
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	row, err := model.FindArkPrefixCache(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), id)
	if err != nil {
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	deleteArkCache(c, row)
}

func DeleteArkCacheAsRoot(c *gin.Context) {
	id := c.Param("response_id")
	if !model.IsArkManagedCacheID(id) || len(id) > 64 {
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	row, err := model.FindArkCacheForCleanup(c.Request.Context(), id)
	if err != nil {
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	deleteArkCache(c, row)
	status := c.Writer.Status()
	actor := c.GetInt("id")
	model.RecordOperationAuditLog(row.UserID, c.GetInt("role"), "Ark cache administrative cleanup", c.ClientIP(),
		"ark_cache_cleanup", map[string]any{"cache_id": row.ID, "app_id": row.TokenID},
		&model.AuditAdminInfo{AdminID: actor, AdminRole: c.GetInt("role")},
		&model.AuditRequestInfo{Method: c.Request.Method, Route: c.FullPath(), Status: status, Success: status == http.StatusOK}, c)
}

func deleteArkCache(c *gin.Context, row model.ArkPrefixCache) {
	switch row.State {
	case model.ArkCacheDeleted:
		c.JSON(http.StatusOK, gin.H{"id": row.ID, "object": "response", "deleted": true})
		return
	case model.ArkCacheDeleting:
		arkCacheError(c, http.StatusConflict, "cache_delete_outcome_unknown")
		return
	case model.ArkCacheActive, model.ArkCacheUnknown:
		if row.ProviderID != "" {
			break
		}
		fallthrough
	default:
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	credential := service.ArkCacheResourceCredential(row)
	if credential == "" {
		arkCacheError(c, http.StatusServiceUnavailable, "cache_unavailable")
		return
	}
	ctx := c.Request.Context()
	if err := model.BeginArkPrefixCacheDelete(ctx, row, common.GetTimestamp()); err != nil {
		arkCacheStateError(c, err)
		return
	}
	deleted, err := service.SendArkCacheDelete(ctx, row, credential)
	if err != nil {
		arkCacheError(c, http.StatusConflict, "cache_delete_outcome_unknown")
		return
	}
	if !deleted {
		arkCacheError(c, http.StatusNotFound, "cache_not_found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": row.ID, "object": "response", "deleted": true})
}

func RecordArkPrefixCacheHour(c *gin.Context) {
	var input struct {
		CacheID  string `json:"cache_id"`
		Hour     int64  `json:"hour_start"`
		Tokens   *int   `json:"storage_tokens"`
		Evidence string `json:"bill_detail_id"`
	}
	if common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, 4096), &input) != nil || !model.IsArkManagedCacheID(input.CacheID) ||
		len(input.CacheID) > 64 || input.Tokens == nil || strings.TrimSpace(input.Evidence) == "" || len(input.Evidence) > 191 {
		arkCacheError(c, http.StatusBadRequest, "invalid_storage_evidence")
		return
	}
	record := model.ArkPrefixCacheHour{CacheID: input.CacheID, HourStart: input.Hour, Tokens: *input.Tokens,
		Evidence: service.ArkCacheDigest(strings.TrimSpace(input.Evidence)), OperatorID: c.GetInt("id")}
	if err := model.RecordArkPrefixCacheHour(c.Request.Context(), record, common.GetTimestamp()); err != nil {
		if errors.Is(err, model.ErrArkCacheConflict) {
			arkCacheError(c, http.StatusConflict, "storage_evidence_conflicts_with_frozen_facts")
			return
		}
		arkCacheStateError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
