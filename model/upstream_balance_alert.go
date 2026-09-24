package model

import (
	"context"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm/clause"
)

const SystemTaskTypeUpstreamBalanceAlert = "upstream_balance_alert"

// Only delivery coordination is persisted here, never credentials or provider
// balances. The ID binds a connection fingerprint to one recipient.
type UpstreamBalanceAlertDelivery struct {
	ID           string `gorm:"type:varchar(64);primaryKey"`
	ConnectionID string `gorm:"type:varchar(64);index"`
	LastSentAt   int64  `gorm:"bigint"`
	LeaseUntil   int64  `gorm:"bigint"`
	LeaseOwner   string `gorm:"type:varchar(128)"`
}

// Report recipients are shared with hourly reports; the hourly report enable
// switch does not disable balance alerts. Read the database again before send.
func GetUpstreamBalanceAlertRecipients(ctx context.Context) ([]string, error) {
	var option Option
	if err := DB.WithContext(ctx).Where(&Option{Key: "error_report_setting.recipients"}).Find(&option).Error; err != nil {
		return nil, err
	}
	return operation_setting.ValidateErrorReportRecipients(option.Value)
}

func ClaimUpstreamBalanceAlert(ctx context.Context, connectionID, id, owner string, now, repeatAfter int64) (bool, error) {
	row := UpstreamBalanceAlertDelivery{ID: id, ConnectionID: connectionID}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return false, err
	}
	result := DB.WithContext(ctx).Model(&UpstreamBalanceAlertDelivery{}).
		Where("id = ? AND lease_until <= ? AND (last_sent_at = 0 OR last_sent_at <= ?)", id, now, now-repeatAfter).
		Updates(map[string]any{"lease_until": now + 120, "lease_owner": owner})
	return result.RowsAffected == 1, result.Error
}

func FinishUpstreamBalanceAlert(ctx context.Context, id, owner string, sentAt int64) error {
	updates := map[string]any{"lease_until": 0, "lease_owner": ""}
	if sentAt > 0 {
		updates["last_sent_at"] = sentAt
	}
	result := DB.WithContext(ctx).Model(&UpstreamBalanceAlertDelivery{}).Where("id = ? AND lease_owner = ?", id, owner).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrSystemTaskLockLost
	}
	return nil
}

func ResetRecoveredUpstreamBalanceAlert(ctx context.Context, connectionID string, now int64) error {
	return DB.WithContext(ctx).Model(&UpstreamBalanceAlertDelivery{}).
		Where("connection_id = ? AND lease_until <= ?", connectionID, now).
		Updates(map[string]any{"last_sent_at": 0}).Error
}
