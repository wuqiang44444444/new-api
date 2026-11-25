package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func ArkPrefixCacheKeyDigest(key string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("create:"+key)))
}

const (
	ArkPrefixCacheIDPrefix   = "resp_ark_prefix_"
	ArkSessionCacheIDPrefix  = "resp_ark_session_"
	ArkCacheSending          = "sending"
	ArkCacheUnknown          = "unknown"
	ArkCacheRejected         = "rejected"
	ArkCacheActive           = "active"
	ArkCacheDeleting         = "deleting"
	ArkCacheDeleted          = "deleted"
	ArkCacheAbsent           = "absent"
	ArkCacheInferencePending = "infer_pending"
)

func IsArkManagedCacheID(id string) bool {
	return strings.HasPrefix(id, ArkPrefixCacheIDPrefix) || strings.HasPrefix(id, ArkSessionCacheIDPrefix)
}

func ArkSessionCacheKeyDigest(key string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("session:"+key)))
}

var (
	ErrArkCacheConflict = errors.New("cache idempotency conflict")
	ErrArkCacheLimit    = errors.New("cache admission limit reached")
	ErrArkCacheFunds    = errors.New("insufficient cache storage quota")
	ErrArkCacheBusy     = errors.New("cache operation unresolved")
	ErrArkCacheMeter    = errors.New("cache storage metering paused")
	ErrArkCacheExpired  = errors.New("cache expiry reached before send permission")
)

// ArkPrefixCache freezes one upstream response write and its funding scope.
// A child response can carry its own storage evidence, but is never a prefix.
type ArkPrefixCache struct {
	ID                 string `gorm:"type:varchar(64);primaryKey"`
	UserID             int    `gorm:"uniqueIndex:idx_ark_cache_claim,priority:1;index:idx_ark_cache_user_created,priority:1"`
	TokenID            int    `gorm:"uniqueIndex:idx_ark_cache_claim,priority:2;index:idx_ark_cache_live,priority:1"`
	KeyDigest          string `gorm:"type:char(64);uniqueIndex:idx_ark_cache_claim,priority:3"`
	RequestDigest      string `gorm:"type:char(64)"`
	ParentID           string `gorm:"type:varchar(64);index"`
	SessionRound       int
	SessionNextID      string `gorm:"type:varchar(64)"`
	Model              string `gorm:"type:varchar(191)"`
	GroupName          string `gorm:"type:varchar(191)"`
	Username           string `gorm:"type:varchar(191)"`
	TokenName          string `gorm:"type:varchar(191)"`
	ChannelID          int    `gorm:"index:idx_ark_cache_channel_writes,priority:1"`
	Endpoint           string `gorm:"type:varchar(128)"`
	Credential         string `gorm:"type:char(64)"`
	ProviderID         string `gorm:"type:varchar(191)"`
	State              string `gorm:"type:varchar(16);index:idx_ark_cache_delete_state,priority:1;index:idx_ark_cache_stale_send,priority:1"`
	RejectedStatus     int
	SentAt             int64 `gorm:"index:idx_ark_cache_channel_writes,priority:2;index:idx_ark_cache_stale_send,priority:2"`
	CreatedAt          int64 `gorm:"index:idx_ark_cache_user_created,priority:2"`
	ExpiresAt          int64 `gorm:"index:idx_ark_cache_live,priority:2"`
	RequestedUntil     int64
	DeletedAt          int64
	InputTokens        int
	OutputTokens       int
	ReadTokens         int
	PricePerMTokHr     string `gorm:"type:varchar(64)"`
	StorageExpr        string `gorm:"type:text"`
	StorageTier        string `gorm:"type:varchar(191)"`
	StorageCNYPerUSD   float64
	QuotaPerUnit       string `gorm:"type:varchar(64)"`
	MeterVersion       string `gorm:"type:varchar(64)"`
	MeterBlocked       bool
	ReuseStorage       string `gorm:"type:varchar(16)"`
	MaxStored          int
	HeldQuota          int
	SettledQuota       int
	SettledTokens      int64
	NextHour           int64
	FundsDeadline      int64
	FundsClosedAt      int64
	ReleasedQuota      int
	FundsOutcome       string `gorm:"type:varchar(16)"`
	NextActionAt       int64  `gorm:"index:idx_ark_cache_due"`
	InferenceHeldQuota int
	InferenceQuota     int
	InferenceLog       string `gorm:"type:text"`
	InferenceSettledAt int64
	InferenceLogDueAt  int64 `gorm:"index:idx_ark_session_log_due"`
	InferenceNextAt    int64 `gorm:"index:idx_ark_cache_inference_due"`
	DeleteRequestedAt  int64
	DeleteCheckAt      int64 `gorm:"index:idx_ark_cache_delete_state,priority:2"`
	AbsentAt           int64
}

func FindArkPrefixCache(ctx context.Context, userID, tokenID int, id string) (row ArkPrefixCache, err error) {
	err = DB.WithContext(ctx).Where("id = ? AND user_id = ? AND token_id = ?", id, userID, tokenID).Take(&row).Error
	return row, err
}

func FindArkPrefixCacheClaim(ctx context.Context, userID, tokenID int, key string) (row ArkPrefixCache, err error) {
	err = DB.WithContext(ctx).Where("user_id = ? AND token_id = ? AND key_digest = ?", userID, tokenID, key).Take(&row).Error
	return row, err
}

// Hold and send permission are committed together. The Key row also
// serializes admissions for the same application on all three databases.
func ClaimArkPrefixCache(ctx context.Context, row *ArkPrefixCache, maxActive, maxHourlyWrites, maxHeld int, clock func() int64) (existing *ArkPrefixCache, err error) {
	now := clock()
	if row.MeterVersion == ArkSessionLocalMeterVersion && !arkSessionLocalBillingAllowed() {
		return nil, ErrArkCacheMeter
	}
	if row.HeldQuota < 0 || row.InferenceHeldQuota < 0 || row.HeldQuota > math.MaxInt32-row.InferenceHeldQuota ||
		(row.HeldQuota > 0 && row.InferenceHeldQuota > 0 && (row.SessionRound == 0 || !arkCacheUsesAutomaticStorage(*row))) {
		return nil, ErrArkCacheFunds
	}
	var token Token
	var parent ArkPrefixCache
	heldQuota := row.HeldQuota + row.InferenceHeldQuota
	var txOptions *sql.TxOptions
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		// The count after the channel lock must see the preceding admission's
		// commit, even when an earlier read took place before it (e.g. MySQL).
		txOptions = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if row.ParentID != "" {
			if err := tx.Model(&ArkPrefixCache{}).Where("id = ? AND user_id = ? AND token_id = ?", row.ParentID, row.UserID, row.TokenID).
				UpdateColumn("next_action_at", gorm.Expr("next_action_at")).Error; err != nil {
				return err
			}
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND token_id = ?", row.ParentID, row.UserID, row.TokenID).Take(&parent).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", row.TokenID, row.UserID).
			UpdateColumn("accessed_time", gorm.Expr("accessed_time")).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", row.TokenID, row.UserID).Take(&token).Error; err != nil {
			return err
		}
		var prior ArkPrefixCache
		err := tx.Where("user_id = ? AND token_id = ? AND key_digest = ?", row.UserID, row.TokenID, row.KeyDigest).Take(&prior).Error
		if err == nil {
			if prior.RequestDigest != row.RequestDigest {
				return ErrArkCacheConflict
			}
			existing = &prior
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.ParentID != "" && (parent.State != ArkCacheActive || parent.ExpiresAt <= now) {
			return ErrArkCacheBusy
		}
		if row.SessionRound > 1 {
			if parent.SessionRound != row.SessionRound-1 || parent.SessionNextID != "" || parent.Credential != row.Credential {
				return ErrArkCacheBusy
			}
			if err := tx.Model(&parent).UpdateColumn("session_next_id", row.ID).Error; err != nil {
				return err
			}
		}
		var active int64
		if err := tx.Model(&ArkPrefixCache{}).Scopes(arkCacheExecutionScope).
			Where("token_id = ? AND expires_at > ? AND state NOT IN ?", row.TokenID, now, []string{ArkCacheRejected, ArkCacheDeleted, ArkCacheAbsent}).Count(&active).Error; err != nil {
			return err
		}
		if active >= int64(maxActive) {
			return ErrArkCacheLimit
		}
		checkHourlyLimit := func(at int64) error {
			hourStart := at - at%3600
			var writes int64
			if err := tx.Model(&ArkPrefixCache{}).Scopes(arkCacheExecutionScope).
				Where("channel_id = ? AND sent_at >= ? AND sent_at < ?", row.ChannelID, hourStart, hourStart+3600).
				Count(&writes).Error; err != nil {
				return err
			}
			if writes >= int64(maxHourlyWrites) {
				return ErrArkCacheLimit
			}
			return nil
		}
		if err := checkHourlyLimit(now); err != nil {
			return err
		}
		if row.HeldQuota > 0 {
			var blocked int64
			if err := tx.Model(&ArkPrefixCache{}).Where("channel_id = ? AND meter_version = ? AND meter_blocked = ?",
				row.ChannelID, row.MeterVersion, true).Count(&blocked).Error; err != nil {
				return err
			}
			if blocked > 0 {
				return ErrArkCacheMeter
			}
			var held int64
			if err := tx.Model(&ArkPrefixCache{}).Where("token_id = ? AND held_quota > 0", row.TokenID).
				Select("COALESCE(SUM(held_quota), 0)").Scan(&held).Error; err != nil {
				return err
			}
			if held+int64(row.HeldQuota) > int64(maxHeld) {
				return ErrArkCacheLimit
			}
		}
		if heldQuota > 0 {
			wallet := tx.Model(&User{}).Where("id = ? AND quota >= ?", row.UserID, heldQuota).
				UpdateColumn("quota", gorm.Expr("quota - ?", heldQuota))
			if wallet.Error != nil {
				return wallet.Error
			}
			if wallet.RowsAffected != 1 {
				return ErrArkCacheFunds
			}
			if !token.UnlimitedQuota && token.RemainQuota < heldQuota {
				return ErrArkCacheFunds
			}
			if err := tx.Model(&token).Updates(map[string]any{
				"remain_quota": gorm.Expr("remain_quota - ?", heldQuota),
				"used_quota":   gorm.Expr("used_quota + ?", heldQuota),
			}).Error; err != nil {
				return err
			}
		}
		if txOptions != nil {
			// Lock the channel after funding rows, matching settlement's lock
			// order. A lost slot rolls back all tentative holds and parent changes.
			if err := lockForUpdate(tx).Select("id").Where("id = ?", row.ChannelID).Take(&Channel{}).Error; err != nil {
				return err
			}
		}
		// SQLite already holds its writer lock. Take the hour from the
		// send permission, including requests that waited across an hour.
		permitAt := max(now, clock())
		if row.ExpiresAt <= permitAt {
			return ErrArkCacheExpired
		}
		if txOptions != nil || permitAt/3600 != now/3600 {
			if err := checkHourlyLimit(permitAt); err != nil {
				return err
			}
		}
		if row.InferenceNextAt > 0 {
			row.InferenceNextAt += permitAt - row.SentAt
		}
		row.SentAt = permitAt
		return tx.Create(row).Error
	}, txOptions)
	if err == nil && existing == nil && heldQuota > 0 {
		invalidateArkCacheBalances(row.UserID, token.Key)
	}
	return existing, err
}

func invalidateArkCacheBalances(userID int, tokenKey string) {
	if err := invalidateUserCache(userID); err != nil {
		common.SysError("failed to invalidate wallet cache after Ark storage adjustment")
	}
	if tokenKey != "" {
		if err := invalidateTokenCacheForMutation(tokenKey); err != nil {
			common.SysError("failed to invalidate token cache after Ark storage adjustment")
		}
	}
}

func CompleteArkPrefixCache(ctx context.Context, row ArkPrefixCache) error {
	fields := map[string]any{
		"state": row.State, "provider_id": row.ProviderID, "created_at": row.CreatedAt,
		"expires_at": row.ExpiresAt, "input_tokens": row.InputTokens, "output_tokens": row.OutputTokens,
		"read_tokens": row.ReadTokens, "next_hour": row.NextHour, "rejected_status": row.RejectedStatus,
	}
	if row.SessionRound > 0 {
		fields["inference_quota"] = row.InferenceQuota
		fields["inference_log"] = row.InferenceLog
		if row.State == ArkCacheRejected || row.State == ArkCacheInferencePending {
			fields["inference_next_at"] = common.GetTimestamp()
		}
	}
	if row.State == ArkCacheRejected && row.HeldQuota > 0 {
		fields["next_action_at"] = common.GetTimestamp()
	}
	if row.SessionRound == 0 && row.State == ArkCacheUnknown && row.ProviderID != "" {
		now := common.GetTimestamp()
		fields["state"], fields["delete_requested_at"], fields["delete_check_at"] = ArkCacheDeleting, now, now
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if row.State == ArkCacheRejected && row.SessionRound > 1 && row.ParentID != "" {
			// Follow admission's parent-before-child lock order. Only this
			// rejected attempt can release the parent's continuation slot.
			if err := tx.Model(&ArkPrefixCache{}).
				Where("id = ? AND user_id = ? AND token_id = ? AND session_next_id = ?", row.ParentID, row.UserID, row.TokenID, row.ID).
				UpdateColumn("session_next_id", "").Error; err != nil {
				return err
			}
		}
		result := tx.Model(&ArkPrefixCache{}).Where("id = ? AND state = ?", row.ID, ArkCacheSending).Updates(fields)
		if result.Error != nil || result.RowsAffected == 1 {
			return result.Error
		}
		// A retry after a lost commit acknowledgement may see the same receipt
		// or its already-settled form. Neither case should write or charge again.
		var saved ArkPrefixCache
		if err := tx.Take(&saved, "id = ?", row.ID).Error; err != nil {
			return err
		}
		if saved.State == ArkCacheRejected && row.State == ArkCacheRejected && saved.RejectedStatus == row.RejectedStatus {
			return nil
		}
		if row.ProviderID != "" && saved.ProviderID == row.ProviderID &&
			(saved.State == row.State || row.State == ArkCacheInferencePending && saved.InferenceSettledAt != 0 ||
				row.State == ArkCacheUnknown && (saved.State == ArkCacheDeleting || saved.State == ArkCacheDeleted || saved.State == ArkCacheAbsent)) {
			return nil
		}
		return ErrArkCacheBusy
	})
}

func BeginArkPrefixCacheDelete(ctx context.Context, row ArkPrefixCache, now int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A conditional write takes the SQLite writer lock before inspecting
		// outstanding sends. Rollback restores the active state on conflict.
		result := tx.Model(&ArkPrefixCache{}).Where("id = ? AND state IN ? AND provider_id <> ''", row.ID, []string{ArkCacheActive, ArkCacheUnknown}).
			Updates(map[string]any{"state": ArkCacheDeleting, "delete_requested_at": now, "delete_check_at": now + 75})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrArkCacheBusy
		}
		var unresolved int64
		if err := tx.Model(&ArkPrefixCache{}).Scopes(arkCacheExecutionScope).Where("parent_id = ? AND expires_at > ? AND state IN ?", row.ID, now,
			[]string{ArkCacheSending, ArkCacheUnknown, ArkCacheInferencePending}).Count(&unresolved).Error; err != nil {
			return err
		}
		if unresolved > 0 {
			return ErrArkCacheBusy
		}
		return nil
	})
}

func ConfirmArkPrefixCacheDelete(ctx context.Context, id string, now int64) error {
	result := DB.WithContext(ctx).Model(&ArkPrefixCache{}).Where("id = ? AND state = ?", id, ArkCacheDeleting).
		Updates(map[string]any{"state": ArkCacheDeleted, "deleted_at": now, "delete_check_at": 0})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrArkCacheBusy
	}
	return nil
}
