package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"gorm.io/gorm"
)

const realPersonConsentTTL = 24 * time.Hour

func CreateConsentPolicy(ctx context.Context, req dto.CreateConsentPolicyRequest) (*model.ConsentPolicy, error) {
	req.Version = strings.TrimSpace(req.Version)
	req.Locale = strings.TrimSpace(req.Locale)
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	if req.Version == "" || req.Locale == "" || req.Title == "" || req.Content == "" {
		return nil, fmt.Errorf("%w: version, locale, title and content are required", ErrInvalidConsentPolicy)
	}
	if len(req.Version) > 64 || len(req.Locale) > 16 || len(req.Title) > 191 {
		return nil, fmt.Errorf("%w: consent policy metadata is too long", ErrInvalidConsentPolicy)
	}
	if req.EffectiveAt == 0 {
		req.EffectiveAt = common.GetTimestamp()
	}
	hash := sha256.Sum256([]byte(req.Content))
	policy := &model.ConsentPolicy{Version: req.Version, Locale: req.Locale, Title: req.Title, Content: req.Content, ContentSHA256: fmt.Sprintf("%x", hash[:]), Status: "draft", EffectiveAt: req.EffectiveAt}
	if err := model.DB.WithContext(ctx).Create(policy).Error; err != nil {
		return nil, err
	}
	return policy, nil
}

func CreateRealPersonAuthorization(ctx context.Context, userID, tokenID int, userGroup, usingGroup string, req dto.CreateRealPersonAuthorizationRequest) (*model.RealPersonAuthorization, string, error) {
	config := asset_setting.Current()
	if !config.ConsentReady() {
		return nil, "", fmt.Errorf("real-person authorization service is unavailable")
	}
	publicURL, err := url.Parse(config.PublicBaseURL)
	if err != nil || publicURL.Scheme != "https" || publicURL.Host == "" {
		return nil, "", fmt.Errorf("real-person authorization service requires an HTTPS public base URL")
	}
	modelName := strings.TrimSpace(req.Model)
	if modelName == "" {
		return nil, "", fmt.Errorf("model is required")
	}
	locale := strings.TrimSpace(req.Locale)
	if locale == "" {
		locale = "zh-CN"
	}
	policy, err := model.GetActiveConsentPolicy(locale)
	if err != nil || policy == nil {
		return nil, "", fmt.Errorf("no active consent policy is available")
	}
	contentHash := sha256.Sum256([]byte(policy.Content))
	if policy.ContentSHA256 == "" || !hmac.Equal([]byte(strings.ToLower(policy.ContentSHA256)), []byte(fmt.Sprintf("%x", contentHash[:]))) {
		return nil, "", fmt.Errorf("active consent policy failed integrity validation")
	}
	channel, profile, fingerprint, err := selectRealPersonAuthorizationChannel(userGroup, usingGroup, modelName)
	if err != nil {
		return nil, "", err
	}
	rawToken, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return nil, "", err
	}
	authorization := &model.RealPersonAuthorization{
		UserID: userID, CreatedByTokenID: tokenID, RequestedModel: modelName,
		ChannelID: channel.Id, CredentialFingerprint: fingerprint, UpstreamProfile: string(profile),
		ProviderProject: channel.GetOtherSettings().AssetProviderProject, Region: channel.GetOtherSettings().AssetRegion,
		PolicyID: policy.ID, PolicyHash: policy.ContentSHA256, Locale: locale,
		Status: model.RealPersonAuthorizationAwaitingConsent, ConsentTokenHash: hashAssetToken(rawToken),
		ConsentTokenExpiresAt: time.Now().Add(realPersonConsentTTL).Unix(),
	}
	if err := model.DB.WithContext(ctx).Create(authorization).Error; err != nil {
		return nil, "", err
	}
	return authorization, rawToken, nil
}

func GetConsentAuthorization(rawToken string) (*model.RealPersonAuthorization, *model.ConsentPolicy, error) {
	authorization, err := model.GetRealPersonAuthorizationByConsentHash(hashAssetToken(rawToken))
	if err != nil || authorization == nil {
		return authorization, nil, err
	}
	var policy model.ConsentPolicy
	if err := model.DB.First(&policy, "id = ?", authorization.PolicyID).Error; err != nil {
		return nil, nil, err
	}
	if policy.ContentSHA256 != authorization.PolicyHash {
		return nil, nil, fmt.Errorf("consent policy integrity check failed")
	}
	return authorization, &policy, nil
}

func AcceptRealPersonConsent(ctx context.Context, rawToken, userAgent, remoteIP string) (*model.RealPersonAuthorization, string, string, error) {
	config := asset_setting.Current()
	if !config.ConsentReady() {
		return nil, "", "", fmt.Errorf("real-person authorization service is unavailable")
	}
	authorization, policy, err := GetConsentAuthorization(rawToken)
	if err != nil || authorization == nil {
		return authorization, "", "", fmt.Errorf("consent link is invalid")
	}
	if authorization.Status != model.RealPersonAuthorizationAwaitingConsent {
		return authorization, "", "", nil
	}
	now := common.GetTimestamp()
	if authorization.ConsentTokenExpiresAt < now {
		expired, err := expireAwaitingRealPersonConsent(ctx, authorization, now)
		if err != nil {
			return nil, "", "", err
		}
		if !expired {
			if err := model.DB.WithContext(ctx).First(authorization, "id = ?", authorization.ID).Error; err != nil {
				return nil, "", "", err
			}
			return authorization, "", "", nil
		}
		return authorization, "", "", fmt.Errorf("consent link has expired")
	}
	receiptToken, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return nil, "", "", err
	}
	evidence := consentEvidenceHMAC(config.ConsentEvidenceHMACKey, authorization.PublicID, policy.ContentSHA256, userAgent, remoteIP, now)
	updates := map[string]any{
		"status": model.RealPersonAuthorizationAwaitingVerification, "adult_confirmed": true,
		"error_code":   "",
		"consented_at": now, "consent_evidence_hmac": evidence, "user_agent": truncateAssetAuditText(userAgent, 512),
		"receipt_token_hash": hashAssetToken(receiptToken), "updated_at": now,
	}
	result := model.DB.WithContext(ctx).Model(&model.RealPersonAuthorization{}).Where("id = ? AND status = ?", authorization.ID, model.RealPersonAuthorizationAwaitingConsent).Updates(updates)
	if result.Error != nil {
		return nil, "", "", result.Error
	}
	if result.RowsAffected == 0 {
		return authorization, "", "", nil
	}
	for key, value := range updates {
		switch key {
		case "status":
			authorization.Status = value.(string)
		case "error_code":
			authorization.ErrorCode = value.(string)
		case "adult_confirmed":
			authorization.AdultConfirmed = true
		case "consented_at":
			authorization.ConsentedAt = value.(int64)
		case "receipt_token_hash":
			hash := value.(string)
			authorization.ReceiptTokenHash = &hash
		}
	}
	h5URL, err := createRealPersonVerificationSession(ctx, authorization)
	if err != nil {
		return authorization, receiptToken, "", err
	}
	return authorization, receiptToken, h5URL, nil
}

func RejectRealPersonConsent(ctx context.Context, rawToken string) error {
	authorization, _, err := GetConsentAuthorization(rawToken)
	if err != nil || authorization == nil {
		return fmt.Errorf("consent link is invalid")
	}
	if authorization.Status != model.RealPersonAuthorizationAwaitingConsent {
		return fmt.Errorf("consent request has already been processed")
	}
	return model.DB.WithContext(ctx).Model(&model.RealPersonAuthorization{}).Where("id = ? AND status = ?", authorization.ID, model.RealPersonAuthorizationAwaitingConsent).Updates(map[string]any{"status": model.RealPersonAuthorizationConsentRejected, "error_code": "real_person_consent_rejected", "updated_at": common.GetTimestamp()}).Error
}

func RefreshRealPersonVerification(ctx context.Context, authorization *model.RealPersonAuthorization) error {
	if authorization == nil || (authorization.Status != model.RealPersonAuthorizationAwaitingVerification && authorization.Status != model.RealPersonAuthorizationVerifying) {
		return nil
	}
	var session model.RealPersonVerificationSession
	if err := model.DB.WithContext(ctx).Where("authorization_id = ?", authorization.ID).Order("id desc").First(&session).Error; err != nil {
		return err
	}
	if session.UpstreamSessionID == "" && session.VerificationHandleCiphertext == "" {
		return refreshUnregisteredVerificationSession(ctx, authorization, &session)
	}
	adapter, channel, err := verificationAdapterForAuthorization(authorization)
	if err != nil {
		return err
	}
	verificationHandle := session.UpstreamSessionID
	if session.VerificationHandleCiphertext != "" {
		verificationHandle, err = common.DecryptShortLivedSecretForScope(
			realPersonVerificationSecretScope(session.AuthorizationID, session.ID),
			session.VerificationHandleCiphertext,
		)
		if err != nil {
			return err
		}
	}
	result, err := adapter.GetVerificationResult(ctx, verificationHandle)
	if err != nil {
		return err
	}
	status := strings.ToLower(result.Status)
	now := common.GetTimestamp()
	authorizationStatus := model.RealPersonAuthorizationVerifying
	authorizationErrorCode := ""
	sessionStatus := "verifying"
	verificationSucceeded := (status == "active" || status == "success" || status == "succeeded" || status == "completed" || status == "group_ready") && result.GroupID != ""
	if verificationSucceeded {
		authorizationStatus = model.RealPersonAuthorizationAuthorized
		sessionStatus = "active"
	} else if status == "failed" || status == "rejected" || status == "expired" {
		authorizationStatus = model.RealPersonAuthorizationFailed
		sessionStatus = status
		switch status {
		case "rejected":
			authorizationErrorCode = "real_person_verification_rejected"
		case "expired":
			authorizationStatus = model.RealPersonAuthorizationExpired
			authorizationErrorCode = "real_person_verification_expired"
		default:
			authorizationErrorCode = "real_person_verification_failed"
		}
	}

	committedStatus := ""
	committedErrorCode := ""
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockRealPersonAuthorization(tx, authorization.ID)
		if err != nil {
			return err
		}
		if current.Status != model.RealPersonAuthorizationAwaitingVerification && current.Status != model.RealPersonAuthorizationVerifying {
			committedStatus = current.Status
			committedErrorCode = current.ErrorCode
			return nil
		}
		updated := tx.Model(&model.RealPersonAuthorization{}).
			Where("id = ? AND status IN ?", current.ID, []string{model.RealPersonAuthorizationAwaitingVerification, model.RealPersonAuthorizationVerifying}).
			Updates(map[string]any{"status": authorizationStatus, "error_code": authorizationErrorCode, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if verificationSucceeded {
			if err := saveAuthorizedRealPersonGroup(tx, current, channel, result.GroupID); err != nil {
				return err
			}
		}
		sessionUpdates := map[string]any{"status": sessionStatus, "error_code": authorizationErrorCode, "last_polled_at": now, "updated_at": now}
		if verificationSucceeded {
			sessionUpdates["upstream_group_id"] = result.GroupID
		}
		if verificationSucceeded || authorizationStatus == model.RealPersonAuthorizationFailed || authorizationStatus == model.RealPersonAuthorizationExpired {
			sessionUpdates["verification_handle_ciphertext"] = ""
			sessionUpdates["h5_url_ciphertext"] = ""
			sessionUpdates["verification_token_hash"] = nil
		}
		if err := tx.Model(&model.RealPersonVerificationSession{}).Where("id = ? AND authorization_id = ?", session.ID, current.ID).Updates(sessionUpdates).Error; err != nil {
			return err
		}
		committedStatus = authorizationStatus
		committedErrorCode = authorizationErrorCode
		return nil
	})
	if err == nil && committedStatus != "" {
		authorization.Status = committedStatus
		authorization.ErrorCode = committedErrorCode
	}
	return err
}

func RetryRealPersonVerification(ctx context.Context, authorization *model.RealPersonAuthorization) (string, error) {
	if authorization == nil || authorization.ConsentedAt == 0 || authorization.RevokedAt != 0 {
		return "", fmt.Errorf("%w: authorization cannot be retried", ErrRealPersonAuthorizationNotRetryable)
	}
	if authorization.Status != model.RealPersonAuthorizationFailed && authorization.Status != model.RealPersonAuthorizationExpired {
		return "", fmt.Errorf("%w: authorization is not retryable", ErrRealPersonAuthorizationNotRetryable)
	}
	return createRealPersonVerificationSession(ctx, authorization)
}

func RevokeRealPersonAuthorization(ctx context.Context, authorization *model.RealPersonAuthorization) error {
	if authorization == nil {
		return fmt.Errorf("authorization not found")
	}
	if authorization.Status == model.RealPersonAuthorizationDeleted {
		return nil
	}
	now := common.GetTimestamp()
	finalStatus := model.RealPersonAuthorizationRevoked
	finalRevokedAt := now
	finalUpdatedAt := now
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockRealPersonAuthorization(tx, authorization.ID)
		if err != nil {
			return err
		}
		if current.Status == model.RealPersonAuthorizationDeleted {
			finalStatus = model.RealPersonAuthorizationDeleted
			finalRevokedAt = current.RevokedAt
			finalUpdatedAt = current.UpdatedAt
			return nil
		}
		if err := tx.Model(&model.RealPersonAuthorization{}).Where("id = ?", current.ID).Updates(map[string]any{"status": model.RealPersonAuthorizationRevoked, "error_code": "", "revoked_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := model.ClearRealPersonVerificationSecretsTx(tx, current.ID); err != nil {
			return err
		}
		var assets []model.Asset
		if err := tx.Where("authorization_id = ? AND deleted_at = ?", current.ID, 0).Find(&assets).Error; err != nil {
			return err
		}
		for i := range assets {
			if err := scheduleAssetDeletionTx(tx, &assets[i], now); err != nil {
				return err
			}
		}
		var groups []model.AssetGroupBinding
		if err := tx.Where("authorization_id = ? AND status <> ?", current.ID, model.AssetBindingStatusDeleted).Find(&groups).Error; err != nil {
			return err
		}
		for i := range groups {
			if err := scheduleAutomaticAssetGroupDeletionTx(tx, &groups[i], now); err != nil {
				return err
			}
		}
		var remainingAssets, remainingGroups int64
		if err := tx.Model(&model.Asset{}).Where("authorization_id = ? AND deleted_at = ?", current.ID, 0).Count(&remainingAssets).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AssetGroupBinding{}).Where("authorization_id = ? AND status <> ?", current.ID, model.AssetBindingStatusDeleted).Count(&remainingGroups).Error; err != nil {
			return err
		}
		if remainingAssets == 0 && remainingGroups == 0 {
			if err := tx.Model(&model.RealPersonAuthorization{}).Where("id = ? AND status = ?", current.ID, model.RealPersonAuthorizationRevoked).Updates(map[string]any{"status": model.RealPersonAuthorizationDeleted, "error_code": "", "updated_at": now}).Error; err != nil {
				return err
			}
			finalStatus = model.RealPersonAuthorizationDeleted
		}
		return nil
	})
	if err == nil {
		authorization.Status = finalStatus
		authorization.ErrorCode = ""
		authorization.RevokedAt = finalRevokedAt
		authorization.UpdatedAt = finalUpdatedAt
	}
	return err
}

func GetReceiptAuthorization(rawToken string) (*model.RealPersonAuthorization, error) {
	return model.GetRealPersonAuthorizationByReceiptHash(hashAssetToken(rawToken))
}

func createRealPersonVerificationSession(ctx context.Context, authorization *model.RealPersonAuthorization) (string, error) {
	adapter, channel, err := verificationAdapterForAuthorization(authorization)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRealPersonVerificationUpstream, err)
	}
	config := asset_setting.Current()
	claimExpiresAt := common.GetTimestamp() + config.CreateUnknownTTLSeconds
	session, err := claimRealPersonVerificationSession(ctx, authorization, claimExpiresAt, config.VerificationPollMaxAttempts)
	if err != nil {
		return "", err
	}
	redirectURL := config.PublicBaseURL + "/consent/real-person/complete?authorization_id=" + url.QueryEscape(authorization.PublicID)
	result, err := adapter.CreateVerificationSession(ctx, assetadapter.VerificationRequest{RedirectURL: redirectURL, ProjectName: "NEWAPI managed real-person asset"})
	validResult := err == nil && (result.SessionID != "" || result.Handle != "") && allowedH5URL(result.H5URL, channel.GetBaseURL(), config.H5AllowedHosts)
	verificationToken := ""
	if validResult {
		verificationToken, err = common.GenerateRandomCharsKey(48)
		if err == nil {
			result.EncryptedHandle, err = common.EncryptShortLivedSecretForScope(
				realPersonVerificationSecretScope(authorization.ID, session.ID),
				result.Handle,
			)
		}
		if err == nil {
			result.EncryptedH5URL, err = common.EncryptShortLivedSecretForScope(
				realPersonVerificationSecretScope(authorization.ID, session.ID),
				result.H5URL,
			)
		}
		if err == nil {
			result.VerificationTokenHash = hashAssetToken(verificationToken)
			result.Handle = ""
			result.H5URL = ""
		}
		validResult = err == nil
	}
	accepted, committedStatus, committedErrorCode, persistErr := persistRealPersonVerificationSessionCreate(
		authorization, session, result, err, validResult, config.VerificationPollMaxAttempts,
	)
	if persistErr != nil {
		common.SysError(fmt.Sprintf("failed to persist verification session create result: authorization_id=%d local_session_id=%d orphan_suspected=true err=%v", authorization.ID, session.ID, persistErr))
		return "", persistErr
	}
	authorization.Status = committedStatus
	authorization.ErrorCode = committedErrorCode
	if err != nil {
		if !assetadapter.IsDefinitiveUpstreamRejection(err) {
			common.SysError(fmt.Sprintf("verification session create outcome remains unknown: authorization_id=%d local_session_id=%d orphan_suspected=true err=%v", authorization.ID, session.ID, err))
		}
		return "", fmt.Errorf("%w: %v", ErrRealPersonVerificationUpstream, err)
	}
	if !validResult {
		if result.SessionID != "" {
			common.SysError(fmt.Sprintf("verification session could not be attached: authorization_id=%d local_session_id=%d upstream_session_present=true orphan_suspected=true", authorization.ID, session.ID))
		}
		return "", fmt.Errorf("%w: upstream returned an invalid verification session", ErrRealPersonVerificationUpstream)
	}
	if !accepted {
		common.SysError(fmt.Sprintf("verification session lost authorization ownership: authorization_id=%d local_session_id=%d upstream_session_present=%t orphan_suspected=true", authorization.ID, session.ID, result.SessionID != ""))
		return "", fmt.Errorf("%w: authorization state changed while creating verification session", ErrRealPersonAuthorizationNotRetryable)
	}
	return config.PublicBaseURL + "/consent/real-person/verify/" + url.PathEscape(verificationToken), nil
}

func OpenRealPersonVerification(ctx context.Context, rawToken string) (string, error) {
	hash := hashAssetToken(strings.TrimSpace(rawToken))
	var session model.RealPersonVerificationSession
	if err := model.DB.WithContext(ctx).
		Where("verification_token_hash = ? AND expires_at > ?", hash, common.GetTimestamp()).
		First(&session).Error; err != nil {
		return "", err
	}
	h5URL, err := common.DecryptShortLivedSecretForScope(
		realPersonVerificationSecretScope(session.AuthorizationID, session.ID),
		session.H5URLCiphertext,
	)
	if err != nil || h5URL == "" {
		return "", fmt.Errorf("verification link is unavailable")
	}
	result := model.DB.WithContext(ctx).Model(&model.RealPersonVerificationSession{}).
		Where("id = ? AND verification_token_hash = ?", session.ID, hash).
		Updates(map[string]any{
			"verification_token_hash": nil,
			"h5_url_ciphertext":       "",
			"updated_at":              common.GetTimestamp(),
		})
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", fmt.Errorf("verification link has already been used")
	}
	return h5URL, nil
}

func verificationAdapterForAuthorization(authorization *model.RealPersonAuthorization) (assetadapter.VerificationAdapter, *model.Channel, error) {
	channel, err := model.GetChannelById(authorization.ChannelID, true)
	if err != nil {
		return nil, nil, err
	}
	key, fingerprint, err := singleChannelCredential(channel)
	if err != nil || fingerprint != authorization.CredentialFingerprint {
		return nil, nil, fmt.Errorf("authorization channel credential changed")
	}
	profile := dto.AssetUpstreamProfile(authorization.UpstreamProfile)
	adapter, err := assetAdapterForChannel(channel, profile, key)
	if err != nil {
		return nil, nil, err
	}
	verification, ok := adapter.(assetadapter.VerificationAdapter)
	if !ok {
		return nil, nil, fmt.Errorf("selected channel does not support verification")
	}
	return verification, channel, nil
}

func selectRealPersonAuthorizationChannel(userGroup, usingGroup, modelName string) (*model.Channel, dto.AssetUpstreamProfile, string, error) {
	candidateGroups := assetCandidateGroups(userGroup, usingGroup)
	abilities := model.GetAllEnableAbilities()
	sort.SliceStable(abilities, func(i, j int) bool {
		left, right := int64(0), int64(0)
		if abilities[i].Priority != nil {
			left = *abilities[i].Priority
		}
		if abilities[j].Priority != nil {
			right = *abilities[j].Priority
		}
		return left > right
	})
	for _, ability := range abilities {
		if !containsAssetGroup(candidateGroups, ability.Group) || ability.Model != modelName {
			continue
		}
		channel, err := model.GetChannelById(ability.ChannelId, true)
		if err != nil || channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeDoubaoVideo {
			continue
		}
		settings := channel.GetOtherSettings()
		profile := settings.AssetUpstreamProfile
		if profile != dto.AssetUpstreamProfileOfficial && profile != dto.AssetUpstreamProfileArk {
			continue
		}
		if !assetProfileMatchesVideoProfile(profile, settings.VideoUpstreamProfile) {
			continue
		}
		_, fingerprint, err := singleChannelCredential(channel)
		if err == nil {
			return channel, profile, fingerprint, nil
		}
	}
	return nil, "", "", fmt.Errorf("no eligible real-person verification channel is available")
}

func saveAuthorizedRealPersonGroup(tx *gorm.DB, authorization *model.RealPersonAuthorization, channel *model.Channel, groupID string) error {
	group := model.AssetGroupBinding{
		UserID: authorization.UserID, AuthorizationID: &authorization.ID,
		ScopeKey:  model.AssetScopeKey(authorization.UserID, &authorization.ID),
		ChannelID: channel.Id, CredentialFingerprint: authorization.CredentialFingerprint,
		UpstreamProfile: authorization.UpstreamProfile, ProviderProject: authorization.ProviderProject,
		Region: authorization.Region, GroupKind: "real_person",
		UpstreamResourceID: groupID, UpstreamGroupID: groupID, Status: model.AssetBindingStatusActive,
	}
	if err := tx.Where("scope_key = ? AND channel_id = ? AND credential_fingerprint = ? AND group_kind = ?", group.ScopeKey, group.ChannelID, group.CredentialFingerprint, group.GroupKind).FirstOrCreate(&group).Error; err != nil {
		return err
	}
	return model.ClaimAssetGroupOwnership(tx, &group, groupID)
}

func scheduleAssetDeletionTx(tx *gorm.DB, asset *model.Asset, now int64) error {
	if err := tx.Model(asset).Updates(map[string]any{"status": model.AssetStatusDeleting, "updated_at": now}).Error; err != nil {
		return err
	}
	var bindings []model.AssetBinding
	if err := tx.Where("asset_id = ? AND status <> ?", asset.ID, model.AssetBindingStatusDeleted).Find(&bindings).Error; err != nil {
		return err
	}
	for i := range bindings {
		bindingID := bindings[i].ID
		if err := tx.Model(&model.AssetBinding{}).Where("id = ?", bindingID).Update("status", model.AssetBindingStatusDeleting).Error; err != nil {
			return err
		}
		job := &model.AssetOperationJob{IdempotencyKey: fmt.Sprintf("delete-binding:%d", bindingID), Kind: "delete_binding", AssetID: &asset.ID, BindingID: &bindingID, Status: model.AssetJobPending}
		if _, err := model.EnsureAssetOperationJob(tx, job, true); err != nil {
			return err
		}
	}
	if len(bindings) == 0 {
		return tx.Model(asset).Updates(map[string]any{"status": model.AssetStatusDeleted, "deleted_at": now, "updated_at": now}).Error
	}
	return nil
}

func hashAssetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:])
}

func consentEvidenceHMAC(key, authorizationID, policyHash, userAgent, remoteIP string, timestamp int64) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(fmt.Sprintf("%s\n%s\n%d\n%s\n%s", authorizationID, policyHash, timestamp, userAgent, remoteIP)))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func allowedH5URL(rawURL, channelBaseURL string, configuredHosts []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	allowed := map[string]struct{}{}
	if base, err := url.Parse(channelBaseURL); err == nil && base.Hostname() != "" {
		allowed[strings.ToLower(base.Hostname())] = struct{}{}
	}
	for _, host := range configuredHosts {
		allowed[strings.ToLower(host)] = struct{}{}
	}
	_, ok := allowed[strings.ToLower(parsed.Hostname())]
	return ok
}

func truncateAssetAuditText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
