package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Video task create attempt funds guarantee (24h warranty refund).
// Contract: docs/80-dev/2026-09-22-视频创建未知结果长期占款问题分析与退款闭环方案.md §4.1/§4.3.
// Customer funds must reach a terminal outcome even when the provider outcome
// stays unknown; provider-side facts are never fabricated.

const (
	TaskCreateFundsGuaranteeSeconds int64 = 24 * 60 * 60
	TaskCreateFundRetryDelaySeconds int64 = 30
	TaskCreateFundScanLimit               = 100

	// ReleaseReason values. verified_rejection = existing provider-verified
	// reject-and-release path; warranty_deadline = 24h guarantee refund;
	// warranty_manual = manual release before the deadline.
	TaskCreateAttemptReleaseVerifiedRejection  = "verified_rejection"
	TaskCreateAttemptReleaseWarrantyDeadline   = "warranty_deadline"
	TaskCreateAttemptReleaseWarrantyManual     = "warranty_manual"
	TaskCreateAttemptReleaseWarrantyLateManual = "warranty_late_manual"

	// Exposure source for attempt-level warranty refunds. ProviderAmount nil
	// means unknown; never fake it with the customer quota.
	ProviderCostExposureSourceTaskCreateAttempt = "task_create_attempt"
)

var videoFundClientProtocols = []string{TaskClientProtocolModelArkV3, TaskClientProtocolKlingV1, TaskClientProtocolJimeng}

var (
	// ErrTaskCreateAttemptFundBlocked: funding source cannot be verified; the
	// row stays pending and alarmed, never guessed into wallet credit.
	ErrTaskCreateAttemptFundBlocked = errors.New("task create attempt refund funding source cannot be verified")

	// ErrTaskCreateAttemptMovedToWarrantyRefund: recover found the row past
	// its funds deadline and released the hold in-tx instead of building a
	// chargeable Task.
	ErrTaskCreateAttemptMovedToWarrantyRefund = errors.New("task create attempt moved to warranty refund after its funds deadline")
)

// TaskCreateAttemptFundsDeadlineDue reports whether the frozen customer funds
// guarantee deadline has passed.
func TaskCreateAttemptFundsDeadlineDue(attempt *TaskCreateAttempt, now int64) bool {
	return attempt != nil && attempt.BillingSource == "wallet" && IsLinkVideoTaskClientProtocol(attempt.ClientProtocol) && attempt.FundsDeadlineAt > 0 && now >= attempt.FundsDeadlineAt
}

// GetTaskCreateAttemptFundDebts returns overdue or retry-due creation holds,
// ordered by effective due time + id so one stuck row cannot block others.
func GetTaskCreateAttemptFundDebts(now int64, limit int) ([]*TaskCreateAttempt, error) {
	if limit <= 0 {
		return nil, nil
	}
	var attempts []*TaskCreateAttempt
	err := DB.
		Where("billing_hold_state = ? AND status IN ?", TaskCreateAttemptBillingHeld,
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded}).
		Where("client_protocol IN ? AND billing_source = ?", videoFundClientProtocols, "wallet").
		Where("COALESCE(NULLIF(fund_retry_at, 0), funds_deadline_at) > 0 AND COALESCE(NULLIF(fund_retry_at, 0), funds_deadline_at) <= ?", now).
		Order("COALESCE(NULLIF(fund_retry_at, 0), funds_deadline_at), id").
		Limit(limit).
		Find(&attempts).Error
	return attempts, err
}

// HasTaskCreateAttemptFundWork reports whether any creation hold needs fund
// work now. It is independent of provider polling switches by design.
func HasTaskCreateAttemptFundWork(now int64) bool {
	var id int64
	err := DB.Model(&TaskCreateAttempt{}).
		Where("billing_hold_state = ? AND status IN ?", TaskCreateAttemptBillingHeld,
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded}).
		Where("client_protocol IN ? AND billing_source = ?", videoFundClientProtocols, "wallet").
		Where("COALESCE(NULLIF(fund_retry_at, 0), funds_deadline_at) > 0 AND COALESCE(NULLIF(fund_retry_at, 0), funds_deadline_at) <= ?", now).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

// MarkTaskCreateAttemptRefundRetry schedules the next refund compensation pass
// and records the failure reason. Never capped: a failed refund stays visible
// and due until it succeeds.
func MarkTaskCreateAttemptRefundRetry(id int64, now int64, failure string) error {
	if id == 0 {
		return errors.New("task create attempt is required")
	}
	if len(failure) > 512 {
		failure = failure[:512]
	}
	result := DB.Model(&TaskCreateAttempt{}).
		Where("id = ? AND billing_hold_state = ? AND status IN ?", id, TaskCreateAttemptBillingHeld,
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded}).
		Updates(map[string]any{
			"fund_retry_at":  now + TaskCreateFundRetryDelaySeconds,
			"refund_failure": failure,
			"updated_at":     now,
		})
	return result.Error
}

// ReleaseTaskCreateAttemptHoldWarranty releases a creation hold under the 24h
// funds guarantee. The business Status is preserved (unknown stays unknown);
// only the funds dimension moves, plus a provider-cost exposure row whose
// amount stays unknown.
func ReleaseTaskCreateAttemptHoldWarranty(id int64, reason string, operatorID int) (*TaskAttemptReleaseResult, error) {
	if id == 0 || !isValidWarrantyReleaseReason(reason) {
		return nil, errors.New("invalid task attempt warranty release")
	}
	result := &TaskAttemptReleaseResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", id).Error; err != nil {
			return err
		}
		released, err := releaseTaskCreateAttemptHoldWarrantyTx(tx, &attempt, reason, operatorID)
		if err != nil {
			return err
		}
		*result = *released
		return nil
	})
	if err != nil {
		return nil, err
	}
	dispatchTaskAttemptReleaseCache(result)
	return result, nil
}

func isValidWarrantyReleaseReason(reason string) bool {
	switch reason {
	case TaskCreateAttemptReleaseWarrantyDeadline,
		TaskCreateAttemptReleaseWarrantyManual, TaskCreateAttemptReleaseWarrantyLateManual:
		return true
	}
	return false
}

// releaseTaskCreateAttemptHoldWarrantyTx performs the warranty release inside
// an open transaction with the attempt row already locked. Idempotent: an
// already released warranty row returns the recorded result without moving
// funds again.
func releaseTaskCreateAttemptHoldWarrantyTx(
	tx *gorm.DB,
	attempt *TaskCreateAttempt,
	reason string,
	operatorID int,
) (*TaskAttemptReleaseResult, error) {
	if !IsLinkVideoTaskClientProtocol(attempt.ClientProtocol) {
		return nil, errors.New("fund guarantee only supports video attempts")
	}
	if attempt.BillingHoldState == TaskCreateAttemptBillingReleased {
		if attempt.RefundCompletedAt > 0 {
			return &TaskAttemptReleaseResult{
				UserID:        attempt.UserID,
				BillingSource: attempt.BillingSource,
			}, nil
		}
		return nil, errors.New("task create attempt was released with a different outcome")
	}
	if attempt.BillingHoldState != TaskCreateAttemptBillingHeld ||
		(attempt.Status != TaskCreateAttemptSending &&
			attempt.Status != TaskCreateAttemptUnknown &&
			attempt.Status != TaskCreateAttemptUpstreamSucceeded) {
		return nil, errors.New("task create attempt has no releasable hold")
	}
	if reason == TaskCreateAttemptReleaseWarrantyDeadline && attempt.VideoRefundState == "" && !TaskCreateAttemptFundsDeadlineDue(attempt, common.GetTimestamp()) {
		return nil, errors.New("fund guarantee deadline has not passed")
	}
	if operatorID == 0 {
		// Manual refund intents carry the operator durably on the instruction;
		// derive it here so the release audit trail does not split.
		operatorID = attempt.VideoRefundOperatorID
	}
	released, err := releaseTaskCreateAttemptFundsTx(tx, attempt)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	updates := map[string]any{
		"billing_hold_state":         TaskCreateAttemptBillingReleased,
		"release_reason":             reason,
		"actual_refund_quota":        released.ReleasedQuota,
		"refund_completed_at":        now,
		"fund_retry_at":              0,
		"next_attempt_at":            0,
		"refund_failure":             "",
		"refund_operator_id":         operatorID,
		"frozen_connection_snapshot": nil,
		// recovery_snapshot is kept: it holds the upstream task id and frozen
		// facts needed for late-success verification and exposure audit.
		"updated_at": now,
	}
	if attempt.VideoRefundState == "pending" {
		updates["video_refund_state"] = "refunded"
		updates["video_refund_completed_at"] = now
		updates["video_refund_retry_at"] = 0
		updates["video_refund_quota"] = released.ReleasedQuota
		updates["video_refund_failure"] = ""
	}
	if operatorID > 0 {
		updates["manual_recovery_at"] = now
		updates["manual_recovery_by"] = operatorID

	}
	result := tx.Model(&TaskCreateAttempt{}).
		Where("id = ? AND billing_hold_state = ? AND status IN ?",
			attempt.ID, TaskCreateAttemptBillingHeld,
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded}).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errors.New("task create attempt warranty release lost its state")
	}
	if err := insertProviderCostExposureTx(tx, &ProviderCostExposure{
		SourceKind:            ProviderCostExposureSourceTaskCreateAttempt,
		SourceID:              attempt.AttemptID,
		Reason:                reason,
		UserID:                attempt.UserID,
		ChannelID:             attempt.ChannelID,
		PublicModel:           attempt.PublicModel,
		UpstreamProfile:       attempt.UpstreamProfile,
		CustomerQuotaReleased: released.ReleasedQuota,
	}); err != nil {
		return nil, err
	}
	return released, nil
}

// releaseTaskCreateAttemptFundsTx moves the held customer funds back. Wallet
// refunds use the original wallet; unsupported funding never becomes wallet credit;
// a soft-deleted or hard-deleted token never blocks the customer refund.
func releaseTaskCreateAttemptFundsTx(tx *gorm.DB, attempt *TaskCreateAttempt) (*TaskAttemptReleaseResult, error) {
	released := &TaskAttemptReleaseResult{
		UserID:        attempt.UserID,
		BillingSource: attempt.BillingSource,
	}
	if attempt.HeldQuota <= 0 {
		return released, nil
	}
	var moved int
	switch attempt.BillingSource {
	case "wallet":
		update := tx.Model(&User{}).
			Where("id = ?", attempt.UserID).
			Update("quota", gorm.Expr("quota + ?", attempt.HeldQuota))
		if update.Error != nil {
			return nil, update.Error
		}
		if update.RowsAffected != 1 {
			return nil, errors.New("task attempt warranty wallet release missed the user row")
		}
		moved = attempt.HeldQuota
	default:
		return nil, ErrTaskCreateAttemptFundBlocked
	}
	released.ReleasedQuota = moved
	key, tokenAdjusted, err := releaseTaskAttemptTokenFundsTx(tx, attempt)
	if err != nil {
		return nil, err
	}
	released.TokenKey = key
	released.TokenReleased = tokenAdjusted
	if tokenAdjusted {
		released.TokenReleasedQuota = attempt.HeldQuota
	}
	return released, nil
}

// releaseTaskAttemptTokenFundsTx restores a live token's quota. A soft-deleted
// or hard-deleted token is recognized and skipped: the customer refund never
// rolls back because of token lifecycle changes.
func releaseTaskAttemptTokenFundsTx(tx *gorm.DB, attempt *TaskCreateAttempt) (key string, ok bool, err error) {
	if !attempt.TokenQuotaHeld {
		return "", false, nil
	}
	var token Token
	lookup := lockForUpdate(tx).Unscoped().Where("id = ?", attempt.TokenID).First(&token)
	if lookup.Error != nil {
		if errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			// Hard-deleted token: keep the customer refund; the token quota
			// dimension no longer exists.
			common.SysError("task attempt refund token row is missing: token_id=" + fmt.Sprint(attempt.TokenID))
			return "", false, nil
		}
		return "", false, lookup.Error
	}
	if token.DeletedAt.Valid {
		return "", false, nil
	}
	updates := map[string]any{
		"remain_quota":  gorm.Expr("remain_quota + ?", attempt.HeldQuota),
		"used_quota":    gorm.Expr("used_quota - ?", attempt.HeldQuota),
		"accessed_time": common.GetTimestamp(),
	}
	if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(updates).Error; err != nil {
		return "", false, err
	}
	return token.Key, true, nil
}

// InitTaskCreateAttemptFundsDeadline backfills the frozen 24h funds deadline
// for legacy held rows using created_at + 24h (the documented migration rule
// for records without a verifiable pre-deduction timestamp). Idempotent.
func InitTaskCreateAttemptFundsDeadline() error {
	if !common.IsMasterNode {
		return nil
	}
	result := DB.Model(&TaskCreateAttempt{}).
		Where("client_protocol IN ? AND billing_source = ?", videoFundClientProtocols, "wallet").
		Where("COALESCE(funds_deadline_at, 0) = 0 AND billing_hold_state = ? AND status IN ?", TaskCreateAttemptBillingHeld,
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded}).
		Updates(map[string]any{
			"funds_deadline_at": gorm.Expr("created_at + ?", TaskCreateFundsGuaranteeSeconds),
			"updated_at":        common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("task create attempt funds deadline backfill: %d legacy rows now covered by the 24h guarantee", result.RowsAffected))
	}
	return nil
}
