package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type TaskAttemptReleaseResult struct {
	UserID        int
	TokenKey      string
	ReleasedQuota int
	TokenReleased bool
	BillingSource string
}

type taskCreateBillingSnapshot struct {
	PublicModel string `json:"public_model"`
}

func MarkTaskCreateAttemptUnknown(id int64, upstreamRequestID string) error {
	if id == 0 {
		return errors.New("task create attempt is required")
	}
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if len(upstreamRequestID) > 191 || containsControlCharacter(upstreamRequestID) {
		common.SysError("discarded invalid task create attempt upstream request id")
		upstreamRequestID = ""
	}
	now := common.GetTimestamp()
	updates := map[string]any{
		"status":             TaskCreateAttemptUnknown,
		"outcome_unknown_at": now,
		"next_attempt_at":    now + 30,
		"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
		"updated_at":         now,
	}
	if upstreamRequestID != "" {
		updates["upstream_request_id"] = upstreamRequestID
	}
	result := DB.Model(&TaskCreateAttempt{}).
		Where("id = ? AND status = ? AND billing_hold_state = ?",
			id, TaskCreateAttemptSending, TaskCreateAttemptBillingHeld).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		var attempt TaskCreateAttempt
		if err := DB.Select("status").First(&attempt, "id = ?", id).Error; err != nil {
			return err
		}
		if attempt.Status == TaskCreateAttemptUnknown {
			return nil
		}
		return errors.New("task create attempt is no longer sending")
	}
	return nil
}

func ReleaseTaskCreateAttemptHold(id int64, terminal TaskCreateAttemptStatus) (*TaskAttemptReleaseResult, error) {
	return releaseTaskCreateAttemptHold(id, terminal, taskAttemptReleaseOptions{})
}

type taskAttemptReleaseOptions struct {
	requireUnknown         bool
	deleteIdempotencyClaim bool
	operatorID             int
	auditNote              string
}

func releaseTaskCreateAttemptHold(
	id int64,
	terminal TaskCreateAttemptStatus,
	options taskAttemptReleaseOptions,
) (*TaskAttemptReleaseResult, error) {
	if terminal != TaskCreateAttemptRejected && terminal != TaskCreateAttemptReleasedWithExposure {
		return nil, errors.New("invalid task attempt release status")
	}
	released := &TaskAttemptReleaseResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", id).Error; err != nil {
			return err
		}
		if attempt.BillingHoldState == TaskCreateAttemptBillingReleased {
			if attempt.Status != terminal {
				return errors.New("task create attempt was released with a different outcome")
			}
			released.UserID = attempt.UserID
			released.BillingSource = attempt.BillingSource
			return nil
		}
		if attempt.BillingHoldState != TaskCreateAttemptBillingHeld ||
			(attempt.Status != TaskCreateAttemptSending && attempt.Status != TaskCreateAttemptUnknown) {
			return errors.New("task create attempt has no releasable hold")
		}
		if options.requireUnknown && attempt.Status != TaskCreateAttemptUnknown {
			return errors.New("task create attempt is not an unknown outcome")
		}
		released.UserID = attempt.UserID
		released.ReleasedQuota = attempt.HeldQuota
		released.BillingSource = attempt.BillingSource
		if attempt.HeldQuota > 0 {
			switch attempt.BillingSource {
			case "subscription":
				update := tx.Model(&UserSubscription{}).
					Where("id = ? AND amount_used >= ?", attempt.SubscriptionID, int64(attempt.HeldQuota)).
					Update("amount_used", gorm.Expr("amount_used - ?", int64(attempt.HeldQuota)))
				if update.Error != nil {
					return update.Error
				}
				if update.RowsAffected != 1 {
					return errors.New("task attempt subscription hold could not be released")
				}
				if err := tx.Model(&SubscriptionPreConsumeRecord{}).
					Where("request_id = ? AND status = ?", attempt.AttemptID, "consumed").
					Updates(map[string]any{"status": "refunded", "updated_at": common.GetTimestamp()}).Error; err != nil {
					return err
				}
			default:
				if err := tx.Model(&User{}).Where("id = ?", attempt.UserID).
					Update("quota", gorm.Expr("quota + ?", attempt.HeldQuota)).Error; err != nil {
					return err
				}
			}
			if attempt.TokenQuotaHeld {
				var token Token
				if err := lockForUpdate(tx).First(&token, "id = ?", attempt.TokenID).Error; err != nil {
					return err
				}
				if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
					"remain_quota":  gorm.Expr("remain_quota + ?", attempt.HeldQuota),
					"used_quota":    gorm.Expr("used_quota - ?", attempt.HeldQuota),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
				released.TokenKey = token.Key
				released.TokenReleased = true
			}
		}
		now := common.GetTimestamp()
		updates := map[string]any{
			"status":                     terminal,
			"billing_hold_state":         TaskCreateAttemptBillingReleased,
			"frozen_connection_snapshot": nil,
			"recovery_snapshot":          nil,
			"next_attempt_at":            0,
			"updated_at":                 now,
		}
		if options.operatorID > 0 {
			updates["manual_recovery_at"] = now
			updates["manual_recovery_by"] = options.operatorID
			updates["manual_recovery_note"] = options.auditNote
		}
		update := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status = ? AND billing_hold_state = ?",
				attempt.ID, attempt.Status, TaskCreateAttemptBillingHeld).
			Updates(updates)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("task create attempt release lost its state")
		}
		if err := tx.Model(&TaskAssetAuthorization{}).
			Where("attempt_id = ? AND state = ?", attempt.AttemptID, TaskAssetAuthorizationReserved).
			Updates(map[string]any{"state": TaskAssetAuthorizationClosed, "updated_at": now}).Error; err != nil {
			return err
		}
		if terminal == TaskCreateAttemptRejected {
			if options.deleteIdempotencyClaim {
				if err := tx.Where("attempt_id = ? AND status IN ?", attempt.AttemptID, []string{
					TaskCreateIdempotencyCreating,
					TaskCreateIdempotencyUnknown,
				}).Delete(&TaskCreateIdempotency{}).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&TaskCreateIdempotency{}).
					Where("attempt_id = ? AND status IN ?", attempt.AttemptID, []string{
						TaskCreateIdempotencyCreating,
						TaskCreateIdempotencyUnknown,
					}).
					Updates(map[string]any{
						"attempt_id": "",
						"status":     TaskCreateIdempotencyCreating,
						"updated_at": now,
					}).Error; err != nil {
					return err
				}
			}
		}
		if terminal == TaskCreateAttemptReleasedWithExposure {
			var billing taskCreateBillingSnapshot
			_ = common.Unmarshal(attempt.BillingSnapshot, &billing)
			if err := insertProviderCostExposureTx(tx, &ProviderCostExposure{
				SourceKind:             ProviderCostExposureSourceAttempt,
				SourceID:               attempt.AttemptID,
				Reason:                 string(TaskCreateAttemptReleasedWithExposure),
				UserID:                 attempt.UserID,
				ChannelID:              attempt.ChannelID,
				PublicModel:            billing.PublicModel,
				UpstreamProfile:        attempt.UpstreamProfile,
				LinkImplementationID:   attempt.LinkImplementationID,
				LinkImplementationVer:  attempt.LinkImplementationVersion,
				LinkImplementationHash: attempt.LinkImplementationHash,
				LinkPubSnapshot:        attempt.LinkPubSnapshot,
				CustomerQuotaReleased:  attempt.HeldQuota,
				CreatedAt:              now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if released.ReleasedQuota > 0 {
		if released.BillingSource == "wallet" {
			gopool.Go(func() {
				if err := cacheIncrUserQuota(released.UserID, int64(released.ReleasedQuota)); err != nil {
					common.SysLog("failed to update released task attempt wallet cache: " + err.Error())
				}
			})
		}
		if released.TokenReleased && released.TokenKey != "" && common.RedisEnabled && common.RDB != nil {
			gopool.Go(func() {
				if err := cacheIncrTokenQuota(released.TokenKey, int64(released.ReleasedQuota)); err != nil {
					common.SysLog("failed to update released task attempt token cache: " + err.Error())
				}
			})
		}
	}
	return released, nil
}
