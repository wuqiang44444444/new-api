package model

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// An open incident is unique per node and monitored resource. The nullable
// key lets closed incidents remain available to daily reporting.
type ResourceAlertEpisode struct {
	ID       int64   `gorm:"primaryKey"`
	OpenKey  *string `gorm:"type:char(64);uniqueIndex"`
	Resource string  `gorm:"type:varchar(32)"`
	OpenedAt int64   `gorm:"bigint;index"`
	ClosedAt int64   `gorm:"bigint"`
	Subject  string  `gorm:"type:varchar(255)"`
	BodyHTML string  `gorm:"type:text"`
}

type ResourceAlertDelivery struct {
	ID            int64  `gorm:"primaryKey"`
	EpisodeID     int64  `gorm:"uniqueIndex:idx_resource_alert_recipient"`
	Recipient     string `gorm:"type:varchar(320);uniqueIndex:idx_resource_alert_recipient"`
	Status        string `gorm:"type:varchar(24);index:idx_resource_alert_due,priority:1"`
	NextAttemptAt int64  `gorm:"bigint;index:idx_resource_alert_due,priority:2"`
	Attempts      int
	LeaseUntil    int64  `gorm:"bigint"`
	LeaseOwner    string `gorm:"type:varchar(64)"`
}

// A recorded threshold crossing and its pending deliveries commit together.
func OpenResourceAlert(ctx context.Context, episode ResourceAlertEpisode, recipients []string) (bool, error) {
	opened := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&episode)
		if result.Error != nil || result.RowsAffected != 1 || episode.ID == 0 {
			return result.Error
		}
		opened = true
		for start := 0; start < len(recipients); start += 32 {
			batch := recipients[start:min(start+32, len(recipients))]
			rows := make([]ResourceAlertDelivery, 0, len(batch))
			for _, address := range batch {
				rows = append(rows, ResourceAlertDelivery{EpisodeID: episode.ID, Recipient: address, Status: MailDeliveryPending})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return opened && err == nil, err
}

func CloseResourceAlert(ctx context.Context, key string, at int64) error {
	return DB.WithContext(ctx).Model(&ResourceAlertEpisode{}).Where("open_key = ?", key).
		Updates(map[string]any{"open_key": nil, "closed_at": at}).Error
}

func MarkExpiredResourceAlertDeliveriesUnknown(ctx context.Context, now int64, limit int) error {
	var ids []int64
	if err := DB.WithContext(ctx).Model(&ResourceAlertDelivery{}).
		Where("status = ? AND lease_until <= ?", MailDeliverySending, now).
		Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	return DB.WithContext(ctx).Model(&ResourceAlertDelivery{}).
		Where("id IN ? AND status = ? AND lease_until <= ?", ids, MailDeliverySending, now).
		Updates(map[string]any{"status": MailDeliveryUnknown, "lease_owner": "", "lease_until": 0}).Error
}

func ClaimResourceAlertDelivery(ctx context.Context, owner string, now, leaseUntil int64) (*ResourceAlertDelivery, error) {
	var delivery ResourceAlertDelivery
	pending := []string{MailDeliveryPending, MailDeliveryRetryWait}
	found := DB.WithContext(ctx).Where("status IN ? AND next_attempt_at <= ?", pending, now).
		Order("next_attempt_at, id").Limit(1).Find(&delivery)
	if found.Error != nil || found.RowsAffected == 0 {
		return nil, found.Error
	}
	result := DB.WithContext(ctx).Model(&ResourceAlertDelivery{}).
		Where("id = ? AND status IN ? AND next_attempt_at <= ?", delivery.ID, pending, now).
		Updates(map[string]any{"status": MailDeliverySending, "lease_owner": owner, "lease_until": leaseUntil, "attempts": gorm.Expr("attempts + 1")})
	if result.Error != nil || result.RowsAffected != 1 {
		return nil, result.Error
	}
	delivery.Attempts++
	return &delivery, nil
}

func FinishResourceAlertDelivery(ctx context.Context, id int64, owner, state string, next int64) error {
	result := DB.WithContext(ctx).Model(&ResourceAlertDelivery{}).
		Where("id = ? AND lease_owner = ? AND status = ?", id, owner, MailDeliverySending).
		Updates(map[string]any{"status": state, "lease_owner": "", "lease_until": 0, "next_attempt_at": next})
	if result.Error == nil && result.RowsAffected != 1 {
		return ErrSystemTaskLockLost
	}
	return result.Error
}

func GetResourceAlertEpisode(ctx context.Context, id int64) (ResourceAlertEpisode, error) {
	var episode ResourceAlertEpisode
	err := DB.WithContext(ctx).First(&episode, id).Error
	return episode, err
}
