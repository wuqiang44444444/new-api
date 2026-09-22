package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// VideoRefund is a durable full-refund instruction and audit, not a balance ledger.
// The fields are separate from provider/business status and never exposed by Task JSON.
type VideoRefund struct {
	VideoRefundState       string `json:"-" gorm:"size:24;index"`
	VideoRefundRequestedAt int64  `json:"-"`
	VideoRefundCompletedAt int64  `json:"-"`
	VideoRefundRetryAt     int64  `json:"-" gorm:"index"`
	VideoRefundOperatorID  int    `json:"-"`
	VideoRefundNote        string `json:"-" gorm:"size:1000"`
	VideoRefundQuota       int    `json:"-"`
	VideoRefundWaivedQuota int    `json:"-"`
	VideoRefundFailure     string `json:"-" gorm:"size:128"`
}

var ErrVideoRefundConflict = errors.New("video funding changed; refresh the refund preview")

func IsVideoFundTask(t *Task) bool {
	if t == nil || t.Platform == constant.TaskPlatformAzureBatch || IsImageTask(t) || t.Platform == constant.TaskPlatformSuno || t.Platform == constant.TaskPlatformMidjourney {
		return false
	}
	if IsLinkVideoTaskClientProtocol(t.ClientProtocol) {
		return true
	}
	switch constant.NormalizeTaskAction(t.Action) {
	case constant.TaskActionTextToVideo, constant.TaskActionImageToVideo, constant.TaskActionFirstTailToVideo, constant.TaskActionReferenceToVideo, constant.TaskActionRemix:
		return true
	}
	return false
}

// The preview binds the exact funding owner, amount, state and refund instruction.
// It deliberately excludes secrets, provider payloads and customer media.
func videoRefundVersion(kind string, id int64, quota int, billing any, refund VideoRefund) string {
	b, _ := common.Marshal([]any{kind, id, quota, billing, refund.VideoRefundState, refund.VideoRefundRequestedAt})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// RequestVideoRefund persists intent before executing funds movement. One object
// has one full refund; retries return the existing instruction and cannot recharge.
func RequestVideoRefund(kind string, id int64, version string, operatorID int, note string) error {
	note = strings.TrimSpace(note)
	if operatorID <= 0 || note == "" || len(note) > 1000 {
		return errors.New("refund operator and audit note are required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"video_refund_state": "pending", "video_refund_requested_at": common.GetTimestamp(), "video_refund_retry_at": common.GetTimestamp(), "video_refund_operator_id": operatorID, "video_refund_note": note}
		switch kind {
		case "attempt":
			var a TaskCreateAttempt
			if err := lockForUpdate(tx).First(&a, id).Error; err != nil {
				return err
			}
			if !IsLinkVideoTaskClientProtocol(a.ClientProtocol) {
				return errors.New("not a video attempt")
			}
			if a.BillingSource != "wallet" {
				return ErrTaskCreateAttemptFundBlocked
			}
			if a.VideoRefundState != "" {
				return nil
			}
			if videoAttemptRefundVersion(&a) != version || a.BillingHoldState != TaskCreateAttemptBillingHeld {
				return ErrVideoRefundConflict
			}
			return tx.Model(&a).Updates(updates).Error
		case "task":
			var t Task
			if err := lockForUpdate(tx).First(&t, id).Error; err != nil {
				return err
			}
			if !IsVideoFundTask(&t) {
				return errors.New("not a video task")
			}
			if !IsLinkVideoTaskClientProtocol(t.ClientProtocol) && !t.VideoFundingReady {
				return ErrTaskCreateAttemptFundBlocked
			}
			if t.PrivateData.BillingSource != "" && t.PrivateData.BillingSource != "wallet" {
				return ErrTaskCreateAttemptFundBlocked
			}
			if t.VideoRefundState != "" {
				return nil
			}
			if videoTaskRefundVersion(&t) != version {
				return ErrVideoRefundConflict
			}
			if t.Quota <= 0 && (t.PrivateData.AsyncBilling == nil || t.PrivateData.AsyncBilling.State != TaskBillingStateDebt) {
				return ErrTaskCreateAttemptFundBlocked
			}
			return tx.Model(&t).Updates(updates).Error
		default:
			return errors.New("invalid video funding owner")
		}
	})
}

func videoAttemptRefundVersion(a *TaskCreateAttempt) string {
	return videoRefundVersion("attempt", a.ID, a.HeldQuota, []any{a.Status, a.BillingHoldState, a.BillingSource, a.SubscriptionID}, a.VideoRefund)
}
func videoTaskRefundVersion(t *Task) string {
	return videoRefundVersion("task", t.ID, t.Quota, []any{t.Status, t.BillingState, t.PrivateData.BillingSource, t.PrivateData.SubscriptionId, t.PrivateData.AsyncBilling, t.VideoFundingReady}, t.VideoRefund)
}

func CompleteVideoTaskRefund(id int64) error {
	var release *TaskAttemptReleaseResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var t Task
		if err := lockForUpdate(tx).First(&t, id).Error; err != nil {
			return err
		}
		if !IsVideoFundTask(&t) || t.VideoRefundState == "" {
			return errors.New("video refund was not requested")
		}
		if t.VideoRefundCompletedAt > 0 {
			return nil
		}
		if t.Quota < 0 {
			return ErrTaskCreateAttemptFundBlocked
		}
		a := TaskCreateAttempt{UserID: t.UserId, TokenID: t.PrivateData.TokenId, HeldQuota: t.Quota, BillingSource: t.PrivateData.BillingSource, SubscriptionID: t.PrivateData.SubscriptionId, TokenQuotaHeld: !t.PrivateData.SkipTokenQuota && t.PrivateData.TokenId > 0}
		if a.BillingSource == "" {
			// Legacy native rows predate the persisted funding source and can
			// only be wallet-funded; RequestVideoRefund already refuses any
			// non-empty non-wallet source, so this default never converts an
			// unsupported funding into wallet credit.
			a.BillingSource = "wallet"
		}

		var err error
		release, err = releaseTaskCreateAttemptFundsTx(tx, &a)
		if err != nil {
			return err
		}
		waived := 0
		if async := t.PrivateData.AsyncBilling; async != nil && async.State != TaskBillingStateSettled && async.TargetQuota != nil && *async.TargetQuota > t.Quota {
			waived = *async.TargetQuota - t.Quota
		}
		state := "refunded"
		now := common.GetTimestamp()
		if err := tx.Create(&TaskBillingDelivery{TaskRowID: t.ID, Event: "customer_refund", BeforeQuota: t.Quota, AfterQuota: 0, CreatedAt: now, NextRetryAt: now}).Error; err != nil {
			return err
		}
		if err := insertProviderCostExposureTx(tx, &ProviderCostExposure{SourceKind: "video_refund", SourceID: t.TaskID, Reason: "customer_refund", UserID: t.UserId, ChannelID: t.ChannelId, PublicModel: t.Properties.OriginModelName, CustomerQuotaReleased: release.ReleasedQuota}); err != nil {
			return err
		}
		return tx.Model(&t).Updates(map[string]any{"quota": 0, "video_refund_state": state, "video_refund_completed_at": now, "video_refund_retry_at": 0, "video_refund_failure": "", "video_refund_quota": release.ReleasedQuota, "video_refund_waived_quota": waived}).Error
	})
	if err == nil {
		dispatchTaskAttemptReleaseCache(release)
	}
	return err
}

// This independent queue remains eligible even after provider polling stops.
func PendingVideoRefunds(now int64, limit int) ([]Task, []TaskCreateAttempt, error) {
	var ts []Task
	var as []TaskCreateAttempt
	q := DB.Where("video_refund_state = ? AND video_refund_retry_at <= ?", "pending", now).Order("video_refund_retry_at, id").Limit(limit)
	if err := q.Find(&ts).Error; err != nil {
		return nil, nil, err
	}
	if err := DB.Where("video_refund_state = ? AND video_refund_retry_at <= ?", "pending", now).Order("video_refund_retry_at, id").Limit(limit).Find(&as).Error; err != nil {
		return nil, nil, err
	}
	return ts, as, nil
}

func DeferVideoRefund(kind string, id int64, blocked bool) error {
	var object any = &Task{}
	if kind == "attempt" {
		object = &TaskCreateAttempt{}
	}
	reason := "refund_storage_failure"
	if blocked {
		reason = "funding_evidence_required"
	}
	return DB.Model(object).Where("id = ? AND video_refund_state = ?", id, "pending").Updates(map[string]any{"video_refund_retry_at": common.GetTimestamp() + 30, "video_refund_failure": reason}).Error
}

// Video observations may update provider results after refund, but never restore
// a stale quota or erase the durable customer-refund instruction.
func (t *Task) updateVideoFundObservation(from *TaskStatus) (bool, error) {
	var saved Task
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&saved, t.ID).Error; err != nil {
			return err
		}
		if from != nil && saved.Status != *from {
			return nil
		}
		next := *t
		next.Quota = saved.Quota
		next.VideoRefund = saved.VideoRefund
		next.VideoDeliveryState = saved.VideoDeliveryState
		next.VideoFundingReady = saved.VideoFundingReady
		if saved.VideoRefundState != "" {
			next.PrivateData.AsyncBilling = saved.PrivateData.AsyncBilling
			next.BillingState = saved.BillingState
		}
		if err := tx.Model(&saved).Select("*").Updates(&next).Error; err != nil {
			return err
		}
		saved = next
		applied = true
		return nil
	})
	if err == nil && applied {
		*t = saved
	}
	return applied, err
}

func RecordVideoClientDelivery(id int64, state string) error {
	if state != "write_failed" && state != "server_written" {
		return errors.New("invalid delivery observation")
	}
	return DB.Model(&Task{}).Where("id = ?", id).Update("video_delivery_state", state).Error
}

func MarkVideoTaskFundingReady(id int64) error {
	return DB.Model(&Task{}).Where("id = ?", id).Update("video_funding_ready", true).Error
}
