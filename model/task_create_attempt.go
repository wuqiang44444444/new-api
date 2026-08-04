package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type TaskCreateAttemptStatus string

const (
	TaskCreateAttemptPrepared             TaskCreateAttemptStatus = "prepared"
	TaskCreateAttemptSending              TaskCreateAttemptStatus = "sending"
	TaskCreateAttemptUpstreamSucceeded    TaskCreateAttemptStatus = "upstream_succeeded"
	TaskCreateAttemptComplete             TaskCreateAttemptStatus = "complete"
	TaskCreateAttemptUnknown              TaskCreateAttemptStatus = "unknown"
	TaskCreateAttemptRejected             TaskCreateAttemptStatus = "rejected"
	TaskCreateAttemptReleasedWithExposure TaskCreateAttemptStatus = "released_with_exposure"
)

type TaskCreateAttemptBillingHoldState string

const (
	TaskCreateAttemptBillingUnheld      TaskCreateAttemptBillingHoldState = "unheld"
	TaskCreateAttemptBillingHeld        TaskCreateAttemptBillingHoldState = "held"
	TaskCreateAttemptBillingTransferred TaskCreateAttemptBillingHoldState = "transferred"
	TaskCreateAttemptBillingReleased    TaskCreateAttemptBillingHoldState = "released"
)

type TaskCreateAttempt struct {
	ID                 int64  `json:"-" gorm:"primaryKey"`
	AttemptID          string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	PublicTaskID       string `json:"-" gorm:"type:varchar(191);index"`
	UserID             int    `json:"-" gorm:"index"`
	TokenID            int    `json:"-" gorm:"index"`
	AppID              int    `json:"-" gorm:"index"`
	EndUserSubjectHash string `json:"-" gorm:"type:varchar(64);index"`
	SubscriptionID     int    `json:"-" gorm:"index"`
	ClientProtocol     string `json:"-" gorm:"type:varchar(32);index"`
	RequestHash        string `json:"-" gorm:"type:varchar(64)"`
	// NorthboundContract* are persisted compatibility names for the Link contract identity.
	NorthboundContractID      string                            `json:"-" gorm:"type:varchar(96)"`
	NorthboundContractVersion string                            `json:"-" gorm:"type:varchar(32)"`
	SKUCapabilityVersion      string                            `json:"-" gorm:"type:varchar(64)"`
	SKUCapabilityHash         string                            `json:"-" gorm:"type:varchar(64)"`
	LinkImplementationID      string                            `json:"-" gorm:"type:varchar(128);index"`
	LinkImplementationVersion string                            `json:"-" gorm:"type:varchar(32);index"`
	LinkImplementationHash    string                            `json:"-" gorm:"type:varchar(80)"`
	ChannelID                 int                               `json:"-" gorm:"index"`
	PublicModel               string                            `json:"-" gorm:"type:varchar(191);index"`
	UpstreamProfile           string                            `json:"-" gorm:"type:varchar(64);index"`
	AdapterVersion            string                            `json:"-" gorm:"type:varchar(128)"`
	FrozenConnectionSnapshot  json.RawMessage                   `json:"-" gorm:"type:json"`
	BillingSnapshot           json.RawMessage                   `json:"-" gorm:"type:json"`
	RecoverySnapshot          json.RawMessage                   `json:"-" gorm:"type:json"`
	Status                    TaskCreateAttemptStatus           `json:"-" gorm:"type:varchar(32);index"`
	BillingHoldState          TaskCreateAttemptBillingHoldState `json:"-" gorm:"type:varchar(20);index"`
	BillingSource             string                            `json:"-" gorm:"type:varchar(20);index"`
	HeldQuota                 int                               `json:"-"`
	TokenQuotaTracked         bool                              `json:"-"`
	TokenQuotaHeld            bool                              `json:"-"`
	UpstreamRequestID         string                            `json:"-" gorm:"type:varchar(191);index"`
	UpstreamTaskID            string                            `json:"-" gorm:"type:varchar(191);index"`
	ReconcileAttempts         int                               `json:"-"`
	OutcomeUnknownAt          int64                             `json:"-" gorm:"bigint;index"`
	NextAttemptAt             int64                             `json:"-" gorm:"bigint;index"`
	HoldDeadlineAt            int64                             `json:"-" gorm:"bigint;index"`
	TaskDeadlineAt            int64                             `json:"-" gorm:"bigint;index"`
	ManualRecoveryAt          int64                             `json:"-" gorm:"bigint;index"`
	ManualRecoveryBy          int                               `json:"-"`
	ManualRecoveryNote        string                            `json:"-" gorm:"type:text"`
	CreatedAt                 int64                             `json:"-" gorm:"bigint;index"`
	UpdatedAt                 int64                             `json:"-" gorm:"bigint"`

	LinkPubSnapshot `json:"-" gorm:"embedded"`
}

type TaskCreateAttemptParams struct {
	PublicTaskID              string
	UserID                    int
	TokenID                   int
	AppID                     int
	EndUserSubjectHash        string
	SubscriptionID            int
	ClientProtocol            string
	RequestHash               string
	NorthboundContractID      string
	NorthboundContractVersion string
	SKUCapabilityVersion      string
	SKUCapabilityHash         string
	LinkImplementationID      string
	LinkImplementationVersion string
	LinkImplementationHash    string
	LinkContractNamespace     string
	LinkRouteFamily           string
	PublishedLinkContractSKU  string
	LinkPublicationVersion    int64
	ChannelID                 int
	PublicModel               string
	UpstreamProfile           string
	AdapterVersion            string
	FrozenConnectionSnapshot  json.RawMessage
	BillingSnapshot           json.RawMessage
	HeldQuota                 int
	NextAttemptAt             int64
	HoldDeadlineAt            int64
	TaskDeadlineAt            int64
}

func CreatePreparedTaskAttempt(params TaskCreateAttemptParams) (*TaskCreateAttempt, error) {
	if params.UserID <= 0 || strings.TrimSpace(params.PublicTaskID) == "" ||
		strings.TrimSpace(params.ClientProtocol) == "" || strings.TrimSpace(params.RequestHash) == "" {
		return nil, errors.New("task create attempt identity is incomplete")
	}
	attemptKey, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	attempt := &TaskCreateAttempt{
		AttemptID:                 "attempt_" + attemptKey,
		PublicTaskID:              strings.TrimSpace(params.PublicTaskID),
		UserID:                    params.UserID,
		TokenID:                   params.TokenID,
		AppID:                     params.AppID,
		EndUserSubjectHash:        strings.TrimSpace(params.EndUserSubjectHash),
		SubscriptionID:            params.SubscriptionID,
		ClientProtocol:            strings.TrimSpace(params.ClientProtocol),
		RequestHash:               strings.TrimSpace(params.RequestHash),
		NorthboundContractID:      strings.TrimSpace(params.NorthboundContractID),
		NorthboundContractVersion: strings.TrimSpace(params.NorthboundContractVersion),
		SKUCapabilityVersion:      strings.TrimSpace(params.SKUCapabilityVersion),
		SKUCapabilityHash:         strings.TrimSpace(params.SKUCapabilityHash),
		LinkImplementationID:      strings.TrimSpace(params.LinkImplementationID),
		LinkImplementationVersion: strings.TrimSpace(params.LinkImplementationVersion),
		LinkImplementationHash:    strings.TrimSpace(params.LinkImplementationHash),
		LinkPubSnapshot: LinkPubSnapshot{
			LinkContractNamespace:    strings.TrimSpace(params.LinkContractNamespace),
			LinkRouteFamily:          strings.TrimSpace(params.LinkRouteFamily),
			PublishedLinkContractSKU: strings.TrimSpace(params.PublishedLinkContractSKU),
			LinkPublicationVersion:   params.LinkPublicationVersion,
		},
		ChannelID:                params.ChannelID,
		PublicModel:              strings.TrimSpace(params.PublicModel),
		UpstreamProfile:          strings.TrimSpace(params.UpstreamProfile),
		AdapterVersion:           strings.TrimSpace(params.AdapterVersion),
		FrozenConnectionSnapshot: append(json.RawMessage(nil), params.FrozenConnectionSnapshot...),
		BillingSnapshot:          append(json.RawMessage(nil), params.BillingSnapshot...),
		Status:                   TaskCreateAttemptPrepared,
		BillingHoldState:         TaskCreateAttemptBillingUnheld,
		HeldQuota:                params.HeldQuota,
		NextAttemptAt:            params.NextAttemptAt,
		HoldDeadlineAt:           params.HoldDeadlineAt,
		TaskDeadlineAt:           params.TaskDeadlineAt,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	if err := DB.Create(attempt).Error; err != nil {
		return nil, err
	}
	return attempt, nil
}

func TransitionTaskCreateAttempt(
	tx *gorm.DB,
	id int64,
	fromStatus TaskCreateAttemptStatus,
	fromHold TaskCreateAttemptBillingHoldState,
	toStatus TaskCreateAttemptStatus,
	toHold TaskCreateAttemptBillingHoldState,
	updates map[string]any,
) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if id == 0 || fromStatus == "" || fromHold == "" || toStatus == "" || toHold == "" {
		return false, errors.New("task create attempt transition is incomplete")
	}
	if updates == nil {
		updates = map[string]any{}
	}
	updates["status"] = toStatus
	updates["billing_hold_state"] = toHold
	updates["updated_at"] = common.GetTimestamp()
	result := tx.Model(&TaskCreateAttempt{}).
		Where("id = ? AND status = ? AND billing_hold_state = ?", id, fromStatus, fromHold).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func GetTaskCreateAttemptsDue(now int64, limit int) []*TaskCreateAttempt {
	if limit <= 0 {
		return nil
	}
	var attempts []*TaskCreateAttempt
	err := DB.Where("status IN ? AND next_attempt_at > ? AND next_attempt_at <= ?",
		[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded},
		0,
		now,
	).Order("next_attempt_at, id").Limit(limit).Find(&attempts).Error
	if err != nil {
		return nil
	}
	return attempts
}

func HasTaskCreateAttemptWork(now int64) bool {
	var id int64
	err := DB.Model(&TaskCreateAttempt{}).
		Where("status IN ? AND next_attempt_at > ? AND next_attempt_at <= ?",
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded},
			0,
			now,
		).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func ScheduleTaskCreateAttemptReconcile(id int64, status TaskCreateAttemptStatus, nextAttemptAt int64) error {
	return DB.Model(&TaskCreateAttempt{}).
		Where("id = ? AND status = ?", id, status).
		Updates(map[string]any{
			"reconcile_attempts": gorm.Expr("reconcile_attempts + 1"),
			"next_attempt_at":    nextAttemptAt,
			"updated_at":         common.GetTimestamp(),
		}).Error
}
