package model

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelAlertClosedRecovered     = "recovered"
	ChannelAlertClosedConfigChanged = "configuration_changed"
	ChannelAlertClosedDeleted       = "channel_deleted"
)

type ChannelAlertTaskScope struct {
	Revision string `json:"revision"`
	Slot     int    `json:"slot"`
}

type ChannelAlertEpisode struct {
	ID               int64   `gorm:"primaryKey"`
	OpenKey          *string `gorm:"type:char(64);uniqueIndex"`
	ScopeKey         string  `gorm:"type:char(64);index:idx_channel_alert_scope_closed,priority:1"`
	ChannelID        int     `gorm:"index"`
	ChannelName      string  `gorm:"type:text"`
	Revision         string  `gorm:"type:char(32)"`
	Model            string  `gorm:"type:varchar(128)"`
	Operation        string  `gorm:"type:varchar(48)"`
	Node             string  `gorm:"type:varchar(128)"`
	Kind             string  `gorm:"type:varchar(24)"`
	KeySlot          int
	StatusCode       int
	LastErrorReason  string `gorm:"type:varchar(48)"`
	LastRequestID    string `gorm:"type:varchar(96)"`
	FirstAt          int64
	LastAt           int64
	ConfirmedAt      int64
	MailPending      bool                     `gorm:"index"`
	ClosedAt         int64                    `gorm:"index:idx_channel_alert_scope_closed,priority:2;index"`
	CloseReason      string                   `gorm:"type:varchar(24)"`
	CredentialSlots  []int                    `gorm:"-"`
	MailEvidenceHTML string                   `gorm:"type:text" json:"-"`
	MailSamples      []ChannelMailObservation `gorm:"-" json:"-"`
	MailSamplesJSON  string                   `gorm:"type:text" json:"-"`
	EvidenceCount    int
	RuleVersion      int
	Preview          bool `gorm:"index"`
}

type ChannelAlertDelivery struct {
	ID               int64  `gorm:"primaryKey"`
	EpisodeID        int64  `gorm:"uniqueIndex:idx_channel_alert_recipient"`
	Recipient        string `gorm:"type:varchar(320);uniqueIndex:idx_channel_alert_recipient"`
	Status           string `gorm:"type:varchar(24);index:idx_channel_alert_due,priority:1;index:idx_channel_alert_lease,priority:1"`
	NextAttemptAt    int64  `gorm:"index:idx_channel_alert_due,priority:2"`
	LeaseUntil       int64  `gorm:"index:idx_channel_alert_lease,priority:2"`
	LeaseOwner       string `gorm:"type:varchar(64)"`
	Attempts         int
	StopReason       string `gorm:"type:varchar(64)"`
	QueuedAt         int64
	FirstSubmittedAt int64
	AcceptedAt       int64
}

// ChannelAlertRevision identifies effective channel configuration without sharing credentials.
func ChannelAlertRevision(channel *Channel) string {
	return ChannelAlertRevisionWithAssetKey(channel, "")
}

func ChannelAlertRevisionWithAssetKey(channel *Channel, assetKey string) string {
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	_, _ = io.WriteString(mac, "channel-alert/v1")
	var size [8]byte
	for _, value := range []string{
		strconv.Itoa(channel.Type), channel.Key, channel.GetBaseURL(), channel.Other, channel.Models,
		channel.GetModelMapping(), channel.OtherSettings, stringPointerValue(channel.Setting),
		stringPointerValue(channel.HeaderOverride), stringPointerValue(channel.ParamOverride),
		stringPointerValue(channel.OpenAIOrganization), strconv.FormatBool(channel.ChannelInfo.IsMultiKey), assetKey,
	} {
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = mac.Write(size[:])
		_, _ = io.WriteString(mac, value)
	}
	var sum [sha256.Size]byte
	return hex.EncodeToString(mac.Sum(sum[:0])[:16])
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func CurrentChannelAlertRevision(tx *gorm.DB, channel *Channel, operation, expected string) (string, error) {
	revision := ChannelAlertRevision(channel)
	if revision == expected || !strings.HasPrefix(operation, "asset_") {
		return revision, nil
	}
	credential, err := getChannelAssetCredential(tx, channel.Id)
	if err != nil || credential == nil {
		return revision, err
	}
	return ChannelAlertRevisionWithAssetKey(channel, strings.TrimSpace(credential.AccessKeyID)+"|"+strings.TrimSpace(credential.SecretAccessKey)), nil
}

func ChannelAlertCredentialSlots(channel *Channel) []int {
	var slots []int
	for index, key := range channel.GetKeys() {
		status, set := channel.ChannelInfo.MultiKeyStatusList[index]
		if key != "" && (!set || status == common.ChannelStatusEnabled || status == common.ChannelStatusAutoDisabled) {
			slots = append(slots, index)
		}
	}
	return slots
}

// OpenChannelAlert records the incident and marks it for independent mail preparation.
func OpenChannelAlert(ctx context.Context, episode ChannelAlertEpisode) (bool, error) {
	db := DB.WithContext(ctx)
	var existing ChannelAlertEpisode
	if err := db.Select("id").Where("open_key = ?", episode.OpenKey).Limit(1).Find(&existing).Error; err != nil || existing.ID > 0 {
		return false, err
	}
	var channel Channel
	if err := db.First(&channel, episode.ChannelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	revision, err := CurrentChannelAlertRevision(db, &channel, episode.Operation, episode.Revision)
	if err != nil {
		return false, err
	}
	if revision != episode.Revision || episode.Kind == "credential" && channel.ChannelInfo.IsMultiKey &&
		!slices.Equal(ChannelAlertCredentialSlots(&channel), episode.CredentialSlots) {
		return false, nil
	}
	var superseded int64
	if err := db.Model(&ChannelAlertEpisode{}).
		Where("scope_key = ? AND closed_at >= ?", episode.ScopeKey, episode.FirstAt).Count(&superseded).Error; err != nil || superseded > 0 {
		return false, err
	}
	episode.ChannelName = common.SanitizeTaskDiagnostic(channel.Name)
	episode.MailPending = !episode.Preview
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&episode)
	return result.RowsAffected == 1 && result.Error == nil, result.Error
}

func queueChannelAlertMail(tx *gorm.DB, id int64, recipients []string, queuedAt int64) error {
	for start := 0; start < len(recipients); start += 32 {
		addresses := recipients[start:min(start+32, len(recipients))]
		rows := make([]ChannelAlertDelivery, 0, len(addresses))
		for _, address := range addresses {
			rows = append(rows, ChannelAlertDelivery{EpisodeID: id, Recipient: address, Status: MailDeliveryPending, QueuedAt: queuedAt})
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
	}
	return nil
}

func CloseChannelAlert(ctx context.Context, id, at int64, reason string) error {
	return DB.WithContext(ctx).Model(&ChannelAlertEpisode{}).Where("id = ? AND open_key IS NOT NULL AND last_at < ?", id, at).
		Updates(map[string]any{"open_key": nil, "closed_at": at, "close_reason": reason}).Error
}

func ClaimChannelAlertDelivery(ctx context.Context, owner string, now, leaseUntil int64) (*ChannelAlertDelivery, error) {
	db := DB.WithContext(ctx)
	pending := []string{MailDeliveryPending, MailDeliveryRetryWait}
	// Give a worker that loses the conditional update one chance to claim the next ready delivery.
	for range 2 {
		var delivery ChannelAlertDelivery
		found := db.Where("status IN ? AND next_attempt_at <= ?", pending, now).
			Order("next_attempt_at, id").Limit(1).Find(&delivery)
		if found.Error != nil || found.RowsAffected == 0 {
			return nil, found.Error
		}
		result := db.Model(&ChannelAlertDelivery{}).Where("id = ? AND status IN ? AND next_attempt_at <= ?", delivery.ID, pending, now).
			Updates(map[string]any{"status": MailDeliverySending, "lease_owner": owner, "lease_until": leaseUntil, "attempts": gorm.Expr("attempts + 1")})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		delivery.Attempts++
		return &delivery, nil
	}
	return nil, nil
}

func FinishChannelAlertDelivery(ctx context.Context, delivery ChannelAlertDelivery, owner, status string, next, submitted, accepted int64, stopReason string) error {
	fields := map[string]any{"status": status, "lease_owner": "", "lease_until": 0, "next_attempt_at": next, "stop_reason": stopReason}
	if status == MailDeliveryPending {
		fields["attempts"] = gorm.Expr("attempts - 1")
	}
	if delivery.FirstSubmittedAt == 0 && submitted > 0 {
		fields["first_submitted_at"] = submitted
	}
	if accepted > 0 {
		fields["accepted_at"] = accepted
	}
	result := DB.WithContext(ctx).Model(&ChannelAlertDelivery{}).
		Where("id = ? AND lease_owner = ? AND status = ?", delivery.ID, owner, MailDeliverySending).Updates(fields)
	if result.Error == nil && result.RowsAffected != 1 {
		return ErrSystemTaskLockLost
	}
	return result.Error
}

func ExpireChannelAlertLeases(ctx context.Context, now int64) error {
	db := DB.WithContext(ctx).Model(&ChannelAlertDelivery{})
	var ids []int64
	if err := db.Where("status = ? AND lease_until <= ?", MailDeliverySending, now).
		Order("lease_until, id").Limit(32).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	return DB.WithContext(ctx).Model(&ChannelAlertDelivery{}).Where("id IN ? AND status = ? AND lease_until <= ?", ids, MailDeliverySending, now).
		Updates(map[string]any{"status": MailDeliveryUnknown, "lease_owner": "", "lease_until": 0}).Error
}

func PurgeClosedChannelAlerts(ctx context.Context, cutoff int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []int64
		unresolved := tx.Model(&ChannelAlertDelivery{}).Select("episode_id").
			Where("status NOT IN ?", []string{MailDeliveryAccepted, MailDeliveryStopped})
		if err := tx.Model(&ChannelAlertEpisode{}).Where("closed_at > 0 AND closed_at < ? AND (mail_pending IS NULL OR mail_pending = ?) AND id NOT IN (?)", cutoff, false, unresolved).
			Order("closed_at, id").Limit(64).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
			return err
		}
		if err := tx.Where("episode_id IN ?", ids).Delete(&ChannelAlertDelivery{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Delete(&ChannelAlertEpisode{}).Error
	})
}
