package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type TaskAttemptReleaseResult struct {
	UserID             int
	TokenKey           string
	TokenReleasedQuota int
	ReleasedQuota      int
	TokenReleased      bool
	BillingSource      string
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
	if terminal == TaskCreateAttemptRejected {
		// A verified rejection remains a refund instruction even if the following
		// funds transaction fails or the stale-sending scanner changes status.
		if err := DB.Model(&TaskCreateAttempt{}).Where("id = ? AND client_protocol IN ? AND billing_hold_state = ? AND status IN ?", id, videoFundClientProtocols, TaskCreateAttemptBillingHeld, []TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown}).Updates(map[string]any{"fund_target": "verified_rejection", "fund_retry_at": common.GetTimestamp()}).Error; err != nil {
			return nil, err
		}
	}
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
	if terminal != TaskCreateAttemptRejected {
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
		if IsLinkVideoTaskClientProtocol(attempt.ClientProtocol) && attempt.BillingSource == "wallet" {
			var err error
			released, err = releaseTaskCreateAttemptFundsTx(tx, &attempt)
			if err != nil {
				return err
			}
		} else if attempt.HeldQuota > 0 {
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
				// Soft-deleted tokens must not block the customer refund:
				// recognize the lifecycle state, skip the token quota
				// adjustment, and keep processing customer funds (same
				// semantics as task_billing_atomic.go).
				lookup := lockForUpdate(tx).Unscoped().Where("id = ?", attempt.TokenID).First(&token)
				if lookup.Error != nil {
					if errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
						common.SysError("task attempt refund token row is missing: token_id=" + fmt.Sprintf("%d", attempt.TokenID))
					} else {
						return lookup.Error
					}
				} else if !token.DeletedAt.Valid {
					if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
						"remain_quota":  gorm.Expr("remain_quota + ?", attempt.HeldQuota),
						"used_quota":    gorm.Expr("used_quota - ?", attempt.HeldQuota),
						"accessed_time": common.GetTimestamp(),
					}).Error; err != nil {
						return err
					}
					released.TokenKey = token.Key
					released.TokenReleased = true
					released.TokenReleasedQuota = attempt.HeldQuota
				}
			}
		}
		now := common.GetTimestamp()
		updates := map[string]any{
			"status":                     terminal,
			"billing_hold_state":         TaskCreateAttemptBillingReleased,
			"release_reason":             TaskCreateAttemptReleaseVerifiedRejection,
			"actual_refund_quota":        released.ReleasedQuota,
			"refund_completed_at":        now,
			"fund_retry_at":              0,
			"frozen_connection_snapshot": nil,
			"recovery_snapshot":          nil,
			"next_attempt_at":            0,
			"updated_at":                 now,
		}
		if attempt.VideoRefundState == "pending" {
			updates["video_refund_state"] = "refunded"
			updates["video_refund_completed_at"] = now
			updates["video_refund_retry_at"] = 0
			updates["video_refund_quota"] = released.ReleasedQuota
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	dispatchTaskAttemptReleaseCache(released)
	return released, nil
}

// dispatchTaskAttemptReleaseCache syncs the process caches after a hold
// release transaction has committed. It is best-effort: cache rebuild is
// always possible from the authoritative main database.
func dispatchTaskAttemptReleaseCache(released *TaskAttemptReleaseResult) {
	if released == nil {
		return
	}
	if released.BillingSource == "wallet" && released.ReleasedQuota > 0 {
		gopool.Go(func() {
			if err := cacheIncrUserQuota(released.UserID, int64(released.ReleasedQuota)); err != nil {
				common.SysLog("failed to update released task attempt wallet cache: " + err.Error())
			}
		})
	}
	if released.TokenReleased && released.TokenKey != "" && common.RedisEnabled && common.RDB != nil {
		gopool.Go(func() {
			if err := cacheIncrTokenQuota(released.TokenKey, int64(released.TokenReleasedQuota)); err != nil {
				common.SysLog("failed to update released task attempt token cache: " + err.Error())
			}
		})
	}
}
