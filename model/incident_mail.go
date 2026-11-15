package model

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type pendingIncidentMail struct {
	ID      int64
	Kind    string
	EventAt int64 // Unix milliseconds
}

// RetryIncidentMail prepares independently committed incidents.
// A closed incident may still have mail waiting to be prepared.
func RetryIncidentMail(ctx context.Context, channelAlerts bool) error {
	db := DB.WithContext(ctx)
	var episode, delivery any = &ResourceAlertEpisode{}, &ResourceAlertDelivery{}
	columns := "id, resource AS kind, opened_at * 1000 AS event_at"
	queue := queueResourceAlertMail
	if channelAlerts {
		episode, delivery = &ChannelAlertEpisode{}, &ChannelAlertDelivery{}
		columns = "id, kind, first_at AS event_at"
		queue = queueChannelAlertMail
	}
	var pending []pendingIncidentMail
	err := db.Model(episode).Select(columns).Where("mail_pending = ?", true).
		Order("id").Limit(16).Scan(&pending).Error
	if err != nil || len(pending) == 0 {
		return err
	}
	queuedAt := time.Now().UnixNano()
	prefs, err := loadMailPreferences(db)
	if err != nil {
		return err
	}
	recipients, err := operationsMailRecipients(db)
	if err != nil {
		return err
	}
	// Each incident commits on its own so a row's claim never holds write locks across the batch.
	for _, row := range pending {
		err := db.Transaction(func(tx *gorm.DB) error {
			claim := tx.Model(episode).Where("id = ? AND mail_pending = ?", row.ID, true).UpdateColumn("mail_pending", false)
			if claim.Error != nil {
				return claim.Error
			}
			kind := ResourceMailType(row.Kind)
			if channelAlerts {
				kind = ChannelAlertMailType(row.Kind)
			}
			if claim.RowsAffected == 0 || len(recipients) == 0 || !prefs.Allows(kind, time.UnixMilli(row.EventAt)) {
				return nil
			}
			var existing int64
			if err := tx.Model(delivery).Where("episode_id = ?", row.ID).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				return nil
			}
			return queue(tx, row.ID, recipients, queuedAt)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
