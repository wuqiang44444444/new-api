package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type taskCreateFrozenConnection struct {
	BaseURL           string `json:"base_url"`
	Key               string `json:"key"`
	Proxy             string `json:"proxy,omitempty"`
	Profile           string `json:"profile"`
	CreatePath        string `json:"create_path,omitempty"`
	QueryPathTemplate string `json:"query_path_template,omitempty"`
}

type taskCreateBillingSnapshot struct {
	PublicModel string `json:"public_model"`
	Quota       int    `json:"quota"`
	FreeModel   bool   `json:"free_model"`
}

// PrepareTaskCreateAttempt creates the durable journal before billing and then
// commits billing, authorization reservations and sending in one transaction.
func PrepareTaskCreateAttempt(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if c == nil || info == nil || info.TaskRelayInfo == nil {
		return types.NewError(errors.New("task create attempt context is incomplete"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID) != 0 {
		return nil
	}
	if info.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(info.QuotaClamp, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if info.PriceData.Quota < 0 {
		return types.NewErrorWithStatusCode(errors.New("task quota cannot be negative"), types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	requestHash, err := taskAttemptRequestHash(c, info.ClientProtocol)
	if err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	profile := strings.TrimSpace(string(info.ChannelOtherSettings.VideoUpstreamProfile))
	if profile == "" {
		profile = string(dto.VideoUpstreamProfileOfficial)
	}
	frozen, err := common.Marshal(taskCreateFrozenConnection{
		BaseURL:           info.ChannelBaseUrl,
		Key:               info.ApiKey,
		Proxy:             info.ChannelSetting.Proxy,
		Profile:           profile,
		CreatePath:        info.ChannelOtherSettings.VideoUpstreamCreatePath,
		QueryPathTemplate: info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate,
	})
	if err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	billing, err := common.Marshal(taskCreateBillingSnapshot{
		PublicModel: info.OriginModelName,
		Quota:       info.PriceData.Quota,
		FreeModel:   info.PriceData.FreeModel,
	})
	if err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	contractID, contractVersion := taskAttemptNorthboundContract(c, info.ClientProtocol)
	skuVersion, skuHash := "", ""
	if capability, ok := common.GetContextKeyType[model.VideoSKUCapability](c, constant.ContextKeyResolvedVideoSKUCapability); ok {
		skuVersion, skuHash = capability.Version, capability.ContentHash
	} else if capability, ok := common.GetContextKeyType[model.ImageSKUCapability](c, constant.ContextKeyResolvedImageSKUCapability); ok {
		skuVersion, skuHash = capability.Version, capability.ContentHash
	}
	implementation, err := resolveTaskAttemptLinkImplementation(
		info.PublishedLinkContractSKU,
		info.ChannelOtherSettings.LinkImplementation,
	)
	if err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	now := time.Now()
	taskDeadlineAt := now.Add(24 * time.Hour).Unix()
	if constant.TaskTimeoutMinutes > 0 {
		taskDeadlineAt = now.Add(time.Duration(constant.TaskTimeoutMinutes) * time.Minute).Unix()
	}
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:              info.PublicTaskID,
		UserID:                    info.UserId,
		TokenID:                   info.TokenId,
		AppID:                     info.AppID,
		EndUserSubjectHash:        info.EndUserSubjectHash,
		ClientProtocol:            info.ClientProtocol,
		RequestHash:               requestHash,
		NorthboundContractID:      contractID,
		NorthboundContractVersion: contractVersion,
		SKUCapabilityVersion:      skuVersion,
		SKUCapabilityHash:         skuHash,
		LinkImplementationID:      implementation.ID,
		LinkImplementationVersion: implementation.Version,
		LinkImplementationHash:    implementation.ContentHash,
		LinkContractNamespace:     info.LinkContractNamespace,
		LinkRouteFamily:           info.LinkRouteFamily,
		PublishedLinkContractSKU:  info.PublishedLinkContractSKU,
		LinkPublicationVersion:    info.LinkPublicationVersion,
		ChannelID:                 info.ChannelId,
		PublicModel:               info.OriginModelName,
		UpstreamProfile:           profile,
		AdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(
			info.ChannelType,
			info.ChannelOtherSettings.VideoUpstreamProfile,
		),
		FrozenConnectionSnapshot: frozen,
		BillingSnapshot:          billing,
		NextAttemptAt:            now.Add(2 * time.Minute).Unix(),
		HoldDeadlineAt:           now.Add(24 * time.Hour).Unix(),
		TaskDeadlineAt:           taskDeadlineAt,
	})
	if err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	idempotencyID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskIdempotencyID))
	if err := model.BindTaskCreateIdempotencyAttempt(idempotencyID, attempt.AttemptID); err != nil {
		_, _ = model.TransitionTaskCreateAttempt(
			nil, attempt.ID,
			model.TaskCreateAttemptPrepared, model.TaskCreateAttemptBillingUnheld,
			model.TaskCreateAttemptRejected, model.TaskCreateAttemptBillingReleased,
			map[string]any{"frozen_connection_snapshot": nil},
		)
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	hold, holdErr := holdTaskAttemptForBilling(info, attempt.ID)
	if holdErr != nil {
		_, _ = model.TransitionTaskCreateAttempt(
			nil, attempt.ID,
			model.TaskCreateAttemptPrepared, model.TaskCreateAttemptBillingUnheld,
			model.TaskCreateAttemptRejected, model.TaskCreateAttemptBillingReleased,
			map[string]any{"frozen_connection_snapshot": nil},
		)
		if errors.Is(holdErr, model.ErrTaskAttemptInsufficientQuota) ||
			errors.Is(holdErr, model.ErrTaskAttemptSubscriptionUnavailable) {
			return types.NewErrorWithStatusCode(
				holdErr, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		if errors.Is(holdErr, model.ErrTaskAttemptAuthorizationUnavailable) {
			return types.NewErrorWithStatusCode(
				holdErr, types.ErrorCodeInvalidRequest, http.StatusConflict,
				types.ErrOptionWithSkipRetry(),
			)
		}
		return types.NewError(holdErr, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))
	info.ForcePreConsume = true
	if !info.PriceData.FreeModel {
		info.Billing = billingSessionFromTaskAttempt(info, attempt, hold)
	}
	return nil
}

func holdTaskAttemptForBilling(info *relaycommon.RelayInfo, attemptID int64) (*model.TaskAttemptHoldResult, error) {
	hold := func(source string) (*model.TaskAttemptHoldResult, error) {
		return model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
			AttemptID:      attemptID,
			FundingSource:  source,
			ModelName:      info.OriginModelName,
			Quota:          info.PriceData.Quota,
			AssetPublicIDs: append([]string(nil), info.AssetPublicIDs...),
			IsPlayground:   info.IsPlayground,
		})
	}
	if info.PriceData.FreeModel {
		return hold(BillingSourceWallet)
	}
	preference := common.NormalizeBillingPreference(info.UserSetting.BillingPreference)
	switch preference {
	case "wallet_only":
		return hold(BillingSourceWallet)
	case "subscription_only":
		return hold(BillingSourceSubscription)
	case "wallet_first":
		result, err := hold(BillingSourceWallet)
		if errors.Is(err, model.ErrTaskAttemptInsufficientQuota) {
			return hold(BillingSourceSubscription)
		}
		return result, err
	default:
		hasSubscription, err := model.HasActiveUserSubscription(info.UserId)
		if err != nil {
			return nil, err
		}
		if !hasSubscription {
			return hold(BillingSourceWallet)
		}
		result, err := hold(BillingSourceSubscription)
		if !errors.Is(err, model.ErrTaskAttemptSubscriptionUnavailable) {
			return result, err
		}
		allowWallet, overflowErr := model.UserActiveSubscriptionsAllowWalletOverflow(info.UserId)
		if overflowErr != nil || !allowWallet {
			return nil, err
		}
		return hold(BillingSourceWallet)
	}
}

func billingSessionFromTaskAttempt(
	info *relaycommon.RelayInfo,
	attempt *model.TaskCreateAttempt,
	hold *model.TaskAttemptHoldResult,
) *BillingSession {
	var funding FundingSource
	switch hold.BillingSource {
	case BillingSourceSubscription:
		funding = &SubscriptionFunding{
			requestId:       attempt.AttemptID,
			userId:          info.UserId,
			modelName:       info.OriginModelName,
			amount:          hold.SubscriptionPreConsumed,
			subscriptionId:  hold.SubscriptionID,
			preConsumed:     hold.SubscriptionPreConsumed,
			AmountTotal:     hold.SubscriptionAmountTotal,
			AmountUsedAfter: hold.SubscriptionAmountUsed,
			PlanId:          hold.SubscriptionPlanID,
			PlanTitle:       hold.SubscriptionPlanTitle,
		}
	default:
		funding = &WalletFunding{userId: info.UserId, consumed: hold.HeldQuota}
	}
	tokenConsumed := 0
	if hold.TokenDebited {
		tokenConsumed = hold.HeldQuota
	}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: hold.HeldQuota,
		tokenConsumed:    tokenConsumed,
		taskAttemptID:    attempt.ID,
	}
	if hold.TokenKey != "" {
		info.TokenKey = hold.TokenKey
	}
	session.syncRelayInfo()
	return session
}

func taskAttemptRequestHash(c *gin.Context, protocol string) (string, error) {
	var body []byte
	if contract, ok := relaycommon.GetVideoContractRequest(c); ok {
		encoded, err := common.Marshal(contract)
		if err != nil {
			return "", err
		}
		body = encoded
	} else {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return "", err
		}
		encoded, err := storage.Bytes()
		if err != nil {
			return "", err
		}
		body = encoded
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(c.Request.Method + "\n" + c.Request.URL.Path + "\n" + protocol + "\n"))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func taskAttemptNorthboundContract(c *gin.Context, protocol string) (string, string) {
	if capability, ok := common.GetContextKeyType[model.ImageSKUCapability](c, constant.ContextKeyResolvedImageSKUCapability); ok {
		return capability.ContractID, capability.Version
	}
	if contract, ok := relaycommon.GetVideoContractRequest(c); ok {
		switch contract.ContractID {
		case dto.VideoContractModelArkV3:
			return string(contract.ContractID), "2024-01-01"
		case dto.VideoContractKlingV1:
			return string(contract.ContractID), "v1"
		case dto.VideoContractJimeng:
			return string(contract.ContractID), "2022-08-31"
		default:
			return string(contract.ContractID), "v1"
		}
	}
	return protocol, "v1"
}

func MarkTaskCreateAttemptUpstreamStarted(c *gin.Context) {
	if c == nil {
		return
	}
	if common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID) != 0 {
		common.SetContextKey(c, constant.ContextKeyTaskUpstreamStarted, true)
	}
}

func MarkTaskCreateAttemptOutcomeUnknown(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil {
		return
	}
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	if attemptID == 0 || !common.GetContextKeyBool(c, constant.ContextKeyTaskUpstreamStarted) {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyTaskCreateOutcomeUnknown) {
		return
	}
	if err := model.MarkTaskCreateAttemptUnknown(
		attemptID,
		c.GetString(common.UpstreamRequestIdKey),
	); err != nil {
		common.SysError("mark task create attempt unknown failed: " + err.Error())
	}
	relaycommon.SetTaskCreateDisposition(c, relaycommon.TaskCreateOutcomeUnknown)
	common.SetContextKey(c, constant.ContextKeyTaskCreateOutcomeUnknown, true)
	info.SkipRequestRefund = true
}

func ReleaseRejectedTaskCreateAttempt(c *gin.Context, info *relaycommon.RelayInfo) error {
	if c == nil || info == nil {
		return nil
	}
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	if attemptID == 0 || common.GetContextKeyBool(c, constant.ContextKeyTaskCreateOutcomeUnknown) {
		return nil
	}
	if _, err := model.ReleaseTaskCreateAttemptHold(attemptID, model.TaskCreateAttemptRejected); err != nil {
		return err
	}
	if session, ok := info.Billing.(*BillingSession); ok {
		session.mu.Lock()
		session.refunded = true
		session.mu.Unlock()
	}
	return nil
}

// ResetRejectedTaskCreateAttemptForRetry releases a request that is proven not
// to have reached the provider, then clears only request-local attempt state so
// the existing channel retry loop can establish a fresh frozen attempt.
func ResetRejectedTaskCreateAttemptForRetry(c *gin.Context, info *relaycommon.RelayInfo) error {
	if err := ReleaseRejectedTaskCreateAttempt(c, info); err != nil {
		return err
	}
	common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, 0)
	common.SetContextKey(c, constant.ContextKeyTaskUpstreamStarted, false)
	common.SetContextKey(c, constant.ContextKeyTaskCreateOutcomeUnknown, false)
	relaycommon.SetTaskCreateDisposition(c, relaycommon.TaskCreateSafeToRetryBeforeCreate)
	if info != nil {
		info.Billing = nil
		info.ForcePreConsume = false
		info.SkipRequestRefund = false
	}
	return nil
}

func CompleteSynchronousTaskCreateAttempt(c *gin.Context) error {
	if c == nil {
		return nil
	}
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	if attemptID == 0 {
		return nil
	}
	return model.CompleteTaskCreateAttemptWithoutTask(attemptID)
}
