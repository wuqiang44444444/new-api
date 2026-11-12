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

var errChannelAlertStale = errors.New("stale channel alert evidence")

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
	ID              int64   `gorm:"primaryKey"`
	OpenKey         *string `gorm:"type:char(64);uniqueIndex"`
	ScopeKey        string  `gorm:"type:char(64);index:idx_channel_alert_scope_closed,priority:1"`
	ChannelID       int     `gorm:"index"`
	Revision        string  `gorm:"type:char(32)"`
	Model           string  `gorm:"type:varchar(128)"`
	Operation       string  `gorm:"type:varchar(48)"`
	Node            string  `gorm:"type:varchar(128)"`
	Kind            string  `gorm:"type:varchar(24)"`
	KeySlot         int
	StatusCode      int
	FirstAt         int64
	LastAt          int64
	ConfirmedAt     int64
	ClosedAt        int64  `gorm:"index:idx_channel_alert_scope_closed,priority:2;index"`
	CloseReason     string `gorm:"type:varchar(24)"`
	CredentialSlots []int  `gorm:"-"`
	EvidenceCount   int
	RuleVersion     int
	Preview         bool `gorm:"index"`
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

// OpenChannelAlert atomically commits one incident and its per-recipient work.
func OpenChannelAlert(ctx context.Context, episode ChannelAlertEpisode, recipients []string) (bool, error) {
	var existing ChannelAlertEpisode
	if err := DB.WithContext(ctx).Select("id").Where("open_key = ?", episode.OpenKey).Limit(1).Find(&existing).Error; err != nil || existing.ID > 0 {
		return false, err
	}
	opened := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Writing first also obtains the SQLite writer lock. A stale candidate
		// rolls back this tentative insert together with its open-key claim.
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&episode)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, episode.ChannelID).Error; err != nil {
			return err
		}
		revision, err := CurrentChannelAlertRevision(tx, &channel, episode.Operation, episode.Revision)
		if err != nil {
			return err
		}
		if revision != episode.Revision || episode.Kind == "credential" && channel.ChannelInfo.IsMultiKey &&
			!slices.Equal(ChannelAlertCredentialSlots(&channel), episode.CredentialSlots) {
			return errChannelAlertStale
		}
		var superseded int64
		if err := tx.Model(&ChannelAlertEpisode{}).
			Where("scope_key = ? AND closed_at >= ?", episode.ScopeKey, episode.FirstAt).Count(&superseded).Error; err != nil {
			return err
		}
		if superseded > 0 {
			return errChannelAlertStale
		}
		for start := 0; start < len(recipients); start += 32 {
			addresses := recipients[start:min(start+32, len(recipients))]
			rows := make([]ChannelAlertDelivery, 0, len(addresses))
			for _, address := range addresses {
				rows = append(rows, ChannelAlertDelivery{EpisodeID: episode.ID, Recipient: address, Status: MailDeliveryPending})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		opened = true
		return nil
	})
	if err == errChannelAlertStale || err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return opened && err == nil, err
}

func CloseChannelAlert(ctx context.Context, id, at int64, reason string) error {
	return DB.WithContext(ctx).Model(&ChannelAlertEpisode{}).Where("id = ? AND open_key IS NOT NULL AND last_at < ?", id, at).
		Updates(map[string]any{"open_key": nil, "closed_at": at, "close_reason": reason}).Error
}

func ClaimChannelAlertDelivery(ctx context.Context, owner string, now, leaseUntil int64) (*ChannelAlertDelivery, error) {
	db := DB.WithContext(ctx)
	pending := []string{MailDeliveryPending, MailDeliveryRetryWait}
	var delivery ChannelAlertDelivery
	found := db.Where("status IN ? AND next_attempt_at <= ?", pending, now).
		Order("next_attempt_at, id").Limit(1).Find(&delivery)
	if found.Error != nil || found.RowsAffected == 0 {
		return nil, found.Error
	}
	result := db.Model(&ChannelAlertDelivery{}).Where("id = ? AND status IN ? AND next_attempt_at <= ?", delivery.ID, pending, now).
		Updates(map[string]any{"status": MailDeliverySending, "lease_owner": owner, "lease_until": leaseUntil, "attempts": gorm.Expr("attempts + 1")})
	if result.Error != nil || result.RowsAffected == 0 {
		return nil, result.Error
	}
	delivery.Attempts++
	return &delivery, nil
}

func FinishChannelAlertDelivery(ctx context.Context, delivery ChannelAlertDelivery, owner, status string, next, submitted, accepted int64) error {
	fields := map[string]any{"status": status, "lease_owner": "", "lease_until": 0, "next_attempt_at": next}
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
		if err := tx.Model(&ChannelAlertEpisode{}).Where("closed_at > 0 AND closed_at < ? AND id NOT IN (?)", cutoff, unresolved).
			Order("closed_at, id").Limit(64).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
			return err
		}
		if err := tx.Where("episode_id IN ?", ids).Delete(&ChannelAlertDelivery{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Delete(&ChannelAlertEpisode{}).Error
	})
}
