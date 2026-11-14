package model

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Isolate preference-read errors from the incident transaction. PostgreSQL
// requires rolling back to a savepoint after an unsuccessful SQL statement.
func incidentMailPreferences(tx *gorm.DB) (prefs MailPreferences, err error) {
	err = tx.Transaction(func(read *gorm.DB) error {
		prefs, err = MailPreferencesInTransaction(read)
		return err
	})
	return prefs, err
}

type pendingIncidentMail struct {
	ID      int64
	Kind    string
	EventAt int64 // Unix milliseconds
}

// RetryIncidentMail only considers incidents whose original preparation
// failed. Closed incidents still represent an alert that could not be queued.
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
	return db.Transaction(func(tx *gorm.DB) error {
		prefs, err := MailPreferencesInTransaction(tx)
		if err != nil {
			return err
		}
		recipients, err := operationsMailRecipients(tx)
		if err != nil {
			return err
		}
		now := time.Now().UnixNano()
		for _, row := range pending {
			claim := tx.Model(episode).Where("id = ? AND mail_pending = ?", row.ID, true).UpdateColumn("mail_pending", false)
			if claim.Error != nil {
				return claim.Error
			}
			kind := ResourceMailType(row.Kind)
			if channelAlerts {
				kind = ChannelAlertMailType(row.Kind)
			}
			if claim.RowsAffected == 0 || len(recipients) == 0 || !prefs.Allows(kind, time.UnixMilli(row.EventAt)) {
				continue
			}
			var existing int64
			if err := tx.Model(delivery).Where("episode_id = ?", row.ID).Count(&existing).Error; err != nil {
				return err
			}
			if existing == 0 {
				if err := queue(tx, row.ID, recipients, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
