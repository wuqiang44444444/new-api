package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	videoDeliveryErrorWriteTimeout = 3 * time.Second
	videoDeliveryErrorLeaseSeconds = 30
)

// VideoDeliveryError is the pending error-log projection of a committed Link
// delivery failure or storage incident. The business state owns the outcome.
type VideoDeliveryError struct {
	ID            int64  `gorm:"primaryKey"`
	CreatedAt     int64  `gorm:"autoCreateTime:false"`
	NextAttemptAt int64  `gorm:"index"`
	TaskID        string `gorm:"type:varchar(64)"`
	Part          string `gorm:"type:varchar(31)"`
	UserID        int
	ChannelID     int
	Model         string `gorm:"type:varchar(128)"`
	RequestID     string `gorm:"type:varchar(64)"`
	Stage         string `gorm:"type:varchar(64)"`
	Reason        string `gorm:"type:varchar(64)"`
}

func OpenVideoResultStorageIncident(ctx context.Context, episode ResourceAlertEpisode, event VideoDeliveryError) (bool, error) {
	opened := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		opened, err = OpenResourceAlertInTx(tx, episode, nil)
		if err != nil || !opened {
			return err
		}
		return tx.Create(&event).Error
	})
	return opened && err == nil, err
}

func PendingVideoDeliveryErrors(ctx context.Context, now int64, limit int) ([]VideoDeliveryError, error) {
	var pending []VideoDeliveryError
	err := DB.WithContext(ctx).Where("next_attempt_at <= ?", now).
		Order("next_attempt_at, id").Limit(limit).Find(&pending).Error
	return pending, err
}

// DeliverVideoDeliveryError claims one entry for longer than its bounded
// write attempt. A slow log DB holds no main-DB transaction or row lock.
func DeliverVideoDeliveryError(ctx context.Context, pending VideoDeliveryError, now int64) error {
	ctx, cancel := context.WithTimeout(ctx, videoDeliveryErrorWriteTimeout)
	defer cancel()
	leaseUntil := now + videoDeliveryErrorLeaseSeconds
	claim := DB.WithContext(ctx).Model(&VideoDeliveryError{}).
		Where("id = ? AND next_attempt_at <= ?", pending.ID, now).
		UpdateColumn("next_attempt_at", leaseUntil)
	if claim.Error != nil || claim.RowsAffected == 0 {
		return claim.Error
	}

	detail, err := common.Marshal(struct {
		DeliveryID int64  `json:"video_delivery_id"`
		Part       string `json:"result_part,omitempty"`
	}{pending.ID, pending.Part})
	if err != nil {
		return err
	}
	event := ErrorEvent{
		CreatedAt: pending.CreatedAt, Module: ErrorEventModuleRelay, EventType: clienterrlog.EventVideoDelivery,
		TaskId: pending.TaskID, UserId: pending.UserID, ChannelId: pending.ChannelID, ModelName: pending.Model,
		RequestId: pending.RequestID, Stage: pending.Stage, Reason: pending.Reason, Detail: string(detail),
	}
	if pending.TaskID != "" {
		event.Protocol = TaskClientProtocolModelArkV3
		event.PublicCode = "video_delivery_unavailable"
	}
	if LOG_DB == nil {
		return errors.New("log database unavailable")
	}
	logDB := LOG_DB.WithContext(ctx)
	var existing ErrorEvent
	// The frozen identity also handles a write whose acknowledgement was lost.
	found := logDB.Select("created_at").Where(
		"created_at = ? AND request_id = ? AND event_type = ? AND task_id = ? AND detail = ?",
		event.CreatedAt, event.RequestId, event.EventType, event.TaskId, event.Detail,
	).Limit(1).Find(&existing)
	if found.Error != nil {
		return found.Error
	}
	if found.RowsAffected == 0 {
		if err := logDB.Create(&event).Error; err != nil {
			return err
		}
	}
	return DB.WithContext(ctx).Where("id = ? AND next_attempt_at = ?", pending.ID, leaseUntil).Delete(&VideoDeliveryError{}).Error
}
