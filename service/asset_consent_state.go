package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"gorm.io/gorm"
)

const (
	realPersonVerificationSessionCreating      = "creating"
	realPersonVerificationSessionCreateUnknown = "create_unknown"
	realPersonVerificationSessionOrphaned      = "orphaned"

	realPersonVerificationSessionFailedCode        = "real_person_verification_session_failed"
	realPersonVerificationCreateUnknownCode        = "real_person_verification_session_outcome_unknown"
	realPersonVerificationSessionOrphanedErrorCode = "real_person_verification_session_orphaned"
)

// expireAwaitingRealPersonConsent performs the expiry transition as a CAS so
// it cannot overwrite an accept/reject that won the race.
func expireAwaitingRealPersonConsent(ctx context.Context, authorization *model.RealPersonAuthorization, now int64) (bool, error) {
	result := model.DB.WithContext(ctx).Model(&model.RealPersonAuthorization{}).
		Where("id = ? AND status = ? AND consent_token_expires_at < ?", authorization.ID, model.RealPersonAuthorizationAwaitingConsent, now).
		Updates(map[string]any{
			"status": model.RealPersonAuthorizationExpired, "error_code": "real_person_verification_expired", "updated_at": now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	authorization.Status = model.RealPersonAuthorizationExpired
	authorization.ErrorCode = "real_person_verification_expired"
	authorization.UpdatedAt = now
	return true, nil
}

func realPersonVerificationTerminal(status string) bool {
	switch status {
	case model.RealPersonAuthorizationConsentRejected,
		model.RealPersonAuthorizationAuthorized,
		model.RealPersonAuthorizationFailed,
		model.RealPersonAuthorizationExpired,
		model.RealPersonAuthorizationRevoked,
		model.RealPersonAuthorizationDeleting,
		model.RealPersonAuthorizationDeleted:
		return true
	default:
		return false
	}
}

// claimRealPersonVerificationSession persists the exclusive local creation
// claim before any upstream request is made.
func claimRealPersonVerificationSession(ctx context.Context, authorization *model.RealPersonAuthorization, claimExpiresAt int64, maxAttempts int) (*model.RealPersonVerificationSession, error) {
	session := &model.RealPersonVerificationSession{
		AuthorizationID: authorization.ID,
		Status:          realPersonVerificationSessionCreating,
		ExpiresAt:       claimExpiresAt,
	}
	now := common.GetTimestamp()
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockRealPersonAuthorization(tx, authorization.ID)
		if err != nil {
			return err
		}
		allowedStatuses := []string{
			model.RealPersonAuthorizationAwaitingVerification,
			model.RealPersonAuthorizationFailed,
			model.RealPersonAuthorizationExpired,
		}
		if current.RevokedAt != 0 || (current.Status != model.RealPersonAuthorizationAwaitingVerification && current.Status != model.RealPersonAuthorizationFailed && current.Status != model.RealPersonAuthorizationExpired) {
			return fmt.Errorf("%w: authorization state changed while creating verification session", ErrRealPersonAuthorizationNotRetryable)
		}
		updated := tx.Model(&model.RealPersonAuthorization{}).
			Where("id = ? AND status IN ? AND revoked_at = ?", current.ID, allowedStatuses, int64(0)).
			Updates(map[string]any{"status": model.RealPersonAuthorizationVerifying, "error_code": "", "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("%w: authorization state changed while creating verification session", ErrRealPersonAuthorizationNotRetryable)
		}
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		job := &model.AssetOperationJob{
			IdempotencyKey:  fmt.Sprintf("poll-verification:%d", session.ID),
			Kind:            "poll_verification",
			AuthorizationID: &authorization.ID,
			MaxAttempts:     maxAttempts,
			NextAttemptAt:   claimExpiresAt,
		}
		_, err = model.EnsureAssetOperationJob(tx, job, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	authorization.Status = model.RealPersonAuthorizationVerifying
	authorization.ErrorCode = ""
	authorization.UpdatedAt = now
	return session, nil
}

// persistRealPersonVerificationSessionCreate records every observable upstream
// result. If revoke/another terminal transition wins after the claim, a known
// upstream session is retained as orphaned instead of reviving authorization.
func persistRealPersonVerificationSessionCreate(
	authorization *model.RealPersonAuthorization,
	session *model.RealPersonVerificationSession,
	result assetadapter.VerificationResult,
	createErr error,
	validResult bool,
	maxAttempts int,
) (bool, string, string, error) {
	now := common.GetTimestamp()
	accepted := false
	committedStatus := ""
	committedErrorCode := ""
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		current, err := model.LockRealPersonAuthorization(tx, authorization.ID)
		if err != nil {
			return err
		}
		var latestSession model.RealPersonVerificationSession
		if err := tx.Select("id").Where("authorization_id = ?", current.ID).Order("id desc").First(&latestSession).Error; err != nil {
			return err
		}
		ownsClaim := latestSession.ID == session.ID
		committedStatus = current.Status
		committedErrorCode = current.ErrorCode
		jobKey := fmt.Sprintf("poll-verification:%d", session.ID)

		createOutcomeUnknown := createErr != nil && !assetadapter.IsDefinitiveUpstreamRejection(createErr) && result.SessionID == "" && result.EncryptedHandle == ""
		if createOutcomeUnknown {
			if err := tx.Model(&model.RealPersonVerificationSession{}).
				Where("id = ? AND authorization_id = ?", session.ID, current.ID).
				Updates(map[string]any{
					"status":        realPersonVerificationSessionCreateUnknown,
					"error_code":    realPersonVerificationCreateUnknownCode,
					"error_message": "upstream verification session creation outcome could not be confirmed",
					"updated_at":    now,
				}).Error; err != nil {
				return err
			}
			if ownsClaim && current.Status == model.RealPersonAuthorizationVerifying && current.RevokedAt == 0 {
				return nil
			}
			return model.CompleteQueuedAssetOperationJobTx(tx, jobKey)
		}

		if validResult && ownsClaim && current.Status == model.RealPersonAuthorizationVerifying && current.RevokedAt == 0 {
			if err := tx.Model(&model.RealPersonVerificationSession{}).
				Where("id = ? AND authorization_id = ?", session.ID, current.ID).
				Updates(map[string]any{
					"upstream_session_id": result.SessionID, "upstream_group_id": result.GroupID,
					"verification_handle_ciphertext": result.EncryptedHandle,
					"verification_token_hash":        nullableAssetTokenHash(result.VerificationTokenHash),
					"h5_url_ciphertext":              result.EncryptedH5URL,
					"status":                         "verifying", "expires_at": result.ExpiresAt,
					"error_code": "", "error_message": "", "updated_at": now,
				}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.AssetOperationJob{}).
				Where("idempotency_key = ? AND status IN ?", jobKey, []string{model.AssetJobPending, model.AssetJobFailed}).
				Updates(map[string]any{
					"status": model.AssetJobPending, "attempt_count": 0, "max_attempts": maxAttempts,
					"next_attempt_at": int64(0), "locked_by": "", "locked_until": int64(0),
					"last_error": "", "updated_at": now,
				}).Error; err != nil {
				return err
			}
			accepted = true
			return nil
		}

		sessionStatus := "failed"
		errorCode := realPersonVerificationSessionFailedCode
		errorMessage := "upstream verification session creation failed"
		if result.SessionID != "" || result.EncryptedHandle != "" {
			sessionStatus = realPersonVerificationSessionOrphaned
			errorCode = realPersonVerificationSessionOrphanedErrorCode
			errorMessage = "upstream verification session is no longer attached to an active authorization"
		}
		if err := tx.Model(&model.RealPersonVerificationSession{}).
			Where("id = ? AND authorization_id = ?", session.ID, current.ID).
			Updates(map[string]any{
				"upstream_session_id": result.SessionID, "upstream_group_id": result.GroupID,
				"verification_handle_ciphertext": "",
				"verification_token_hash":        nil,
				"h5_url_ciphertext":              "",
				"status":                         sessionStatus, "expires_at": result.ExpiresAt,
				"error_code": errorCode, "error_message": errorMessage, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		if ownsClaim && current.Status == model.RealPersonAuthorizationVerifying && current.RevokedAt == 0 {
			authorizationErrorCode := errorCode
			if authorizationErrorCode == realPersonVerificationSessionOrphanedErrorCode {
				authorizationErrorCode = realPersonVerificationSessionFailedCode
			}
			updated := tx.Model(&model.RealPersonAuthorization{}).
				Where("id = ? AND status = ? AND revoked_at = ?", current.ID, model.RealPersonAuthorizationVerifying, int64(0)).
				Updates(map[string]any{"status": model.RealPersonAuthorizationFailed, "error_code": authorizationErrorCode, "updated_at": now})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 1 {
				committedStatus = model.RealPersonAuthorizationFailed
				committedErrorCode = authorizationErrorCode
			}
		}
		return model.CompleteQueuedAssetOperationJobTx(tx, jobKey)
	})
	return accepted, committedStatus, committedErrorCode, err
}

func nullableAssetTokenHash(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// refreshUnregisteredVerificationSession closes a durable create claim only
// after its bounded uncertainty window. It never calls upstream without an ID.
func refreshUnregisteredVerificationSession(ctx context.Context, authorization *model.RealPersonAuthorization, session *model.RealPersonVerificationSession) error {
	now := common.GetTimestamp()
	if (session.Status == realPersonVerificationSessionCreating || session.Status == realPersonVerificationSessionCreateUnknown) && session.ExpiresAt > now {
		var current model.RealPersonAuthorization
		if err := model.DB.WithContext(ctx).Select("status", "error_code", "updated_at").First(&current, "id = ?", authorization.ID).Error; err != nil {
			return err
		}
		authorization.Status = current.Status
		authorization.ErrorCode = current.ErrorCode
		authorization.UpdatedAt = current.UpdatedAt
		return nil
	}

	transitioned := false
	committedStatus := ""
	committedErrorCode := ""
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockRealPersonAuthorization(tx, authorization.ID)
		if err != nil {
			return err
		}
		committedStatus = current.Status
		committedErrorCode = current.ErrorCode
		sessionUpdate := tx.Model(&model.RealPersonVerificationSession{}).
			Where("id = ? AND authorization_id = ? AND upstream_session_id = ? AND status IN ?", session.ID, current.ID, "", []string{realPersonVerificationSessionCreating, realPersonVerificationSessionCreateUnknown}).
			Updates(map[string]any{
				"status":         realPersonVerificationSessionCreateUnknown,
				"error_code":     realPersonVerificationCreateUnknownCode,
				"error_message":  "upstream verification session creation outcome could not be confirmed",
				"last_polled_at": now, "updated_at": now,
			})
		if sessionUpdate.Error != nil {
			return sessionUpdate.Error
		}
		if sessionUpdate.RowsAffected == 0 {
			return nil
		}
		transitioned = true
		updated := tx.Model(&model.RealPersonAuthorization{}).
			Where("id = ? AND status IN ? AND revoked_at = ?", current.ID, []string{model.RealPersonAuthorizationAwaitingVerification, model.RealPersonAuthorizationVerifying}, int64(0)).
			Updates(map[string]any{"status": model.RealPersonAuthorizationFailed, "error_code": realPersonVerificationCreateUnknownCode, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 1 {
			committedStatus = model.RealPersonAuthorizationFailed
			committedErrorCode = realPersonVerificationCreateUnknownCode
		}
		return nil
	})
	if err != nil {
		return err
	}
	if committedStatus != "" {
		authorization.Status = committedStatus
		authorization.ErrorCode = committedErrorCode
	}
	if transitioned {
		common.SysError(fmt.Sprintf("verification session create outcome remains unknown: authorization_id=%d session_id=%d orphan_suspected=true", authorization.ID, session.ID))
	}
	return nil
}

func expireRealPersonVerificationPoll(job *model.AssetOperationJob, authorization *model.RealPersonAuthorization) error {
	now := common.GetTimestamp()
	committedStatus := ""
	committedErrorCode := ""
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		current, err := model.LockRealPersonAuthorization(tx, authorization.ID)
		if err != nil {
			return err
		}
		committedStatus = current.Status
		committedErrorCode = current.ErrorCode
		updated := tx.Model(&model.RealPersonAuthorization{}).
			Where("id = ? AND status IN ?", current.ID, []string{model.RealPersonAuthorizationAwaitingVerification, model.RealPersonAuthorizationVerifying}).
			Updates(map[string]any{"status": model.RealPersonAuthorizationExpired, "error_code": "real_person_verification_expired", "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 1 {
			committedStatus = model.RealPersonAuthorizationExpired
			committedErrorCode = "real_person_verification_expired"
		}
		if err := model.ClearRealPersonVerificationSecretsTx(tx, current.ID); err != nil {
			return err
		}
		return model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy)
	})
	if err != nil {
		return err
	}
	authorization.Status = committedStatus
	authorization.ErrorCode = committedErrorCode
	return nil
}
