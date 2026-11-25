package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const ArkCacheBillingMode = "cache_storage"

// ArkPrefixCacheHour is independently observed storage metering for one
// resource and hour. Delivery acknowledgement never changes funding.
type ArkPrefixCacheHour struct {
	ID          int64  `gorm:"primaryKey"`
	CacheID     string `gorm:"type:varchar(64);uniqueIndex:idx_ark_cache_hour,priority:1"`
	HourStart   int64  `gorm:"uniqueIndex:idx_ark_cache_hour,priority:2"`
	Evidence    string `gorm:"type:char(64);uniqueIndex"`
	Tokens      int
	OperatorID  int
	Quota       int
	SettledAt   int64 `gorm:"index:idx_ark_cache_delivery,priority:1"`
	DeliveredAt int64 `gorm:"index:idx_ark_cache_delivery,priority:2"`
	NextRetryAt int64
}

func ArkPrefixCacheQuota(price, quotaPerUnit string, tokenHours int64, reserve bool) (int, error) {
	if tokenHours < 0 {
		return 0, ErrArkCacheConflict
	}
	unitPrice, err := decimal.NewFromString(price)
	if err != nil || !unitPrice.IsPositive() {
		return 0, ErrArkCacheConflict
	}
	rate, err := decimal.NewFromString(quotaPerUnit)
	if err != nil || !rate.IsPositive() {
		return 0, ErrArkCacheConflict
	}
	amount := decimal.NewFromInt(tokenHours).Mul(unitPrice).Mul(rate).Shift(-6)
	if reserve {
		amount = amount.Ceil()
	}
	return common.QuotaFromDecimalStrict(amount)
}

// RecordArkPrefixCacheHour admits reviewed evidence. A retry with identical
// evidence is a no-op. A changed figure requires financial review.
func RecordArkPrefixCacheHour(ctx context.Context, record ArkPrefixCacheHour, now int64) error {
	if record.HourStart <= 0 || record.HourStart%3600 != 0 || record.HourStart > now-3600 || record.Tokens < 0 {
		return ErrArkCacheConflict
	}
	var exceeded bool
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ArkPrefixCache{}).Where("id = ?", record.CacheID).
			UpdateColumn("next_action_at", gorm.Expr("next_action_at")).Error; err != nil {
			return err
		}
		var cache ArkPrefixCache
		if err := lockForUpdate(tx).Take(&cache, "id = ?", record.CacheID).Error; err != nil {
			return err
		}
		if arkCacheUsesAutomaticStorage(cache) {
			return ErrArkCacheConflict
		}
		var previous ArkPrefixCacheHour
		err := tx.Where("cache_id = ? AND hour_start = ?", record.CacheID, record.HourStart).Take(&previous).Error
		if err == nil {
			if previous.Evidence == record.Evidence && previous.Tokens == record.Tokens {
				return nil
			}
			return ErrArkCacheConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if cache.FundsClosedAt != 0 || cache.MeterVersion == "" || cache.CreatedAt == 0 ||
			(cache.SessionRound > 0 && cache.InferenceSettledAt == 0) ||
			(cache.State != ArkCacheActive && cache.State != ArkCacheDeleted && !((cache.State == ArkCacheDeleting || cache.State == ArkCacheAbsent) && now >= cache.ExpiresAt)) ||
			now >= cache.FundsDeadline || record.HourStart+3600 <= cache.CreatedAt || record.HourStart >= arkCacheEnd(cache) {
			return ErrArkCacheConflict
		}
		var reused int64
		if err := tx.Model(&ArkPrefixCacheHour{}).Where("evidence = ?", record.Evidence).Count(&reused).Error; err != nil {
			return err
		}
		if reused != 0 {
			return ErrArkCacheConflict
		}
		if record.Tokens > cache.MaxStored {
			exceeded = true
			return tx.Model(&cache).UpdateColumn("meter_blocked", true).Error
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Model(&cache).UpdateColumn("next_action_at", now).Error
	})
	if err == nil && exceeded {
		return ErrArkCacheMeter
	}
	return err
}

func arkCacheEnd(row ArkPrefixCache) int64 {
	if row.DeletedAt > 0 {
		return min(row.ExpiresAt, row.DeletedAt)
	}
	return row.ExpiresAt
}

func ArkPrefixCacheDue(ctx context.Context, now int64) (rows []ArkPrefixCache, err error) {
	err = DB.WithContext(ctx).Scopes(arkCacheExecutionScope).Where("next_action_at > 0 AND next_action_at <= ?", now).
		Order("next_action_at, id").Limit(50).Find(&rows).Error
	return rows, err
}

func SettleArkPrefixCache(ctx context.Context, id string, now int64) error {
	var releasedUser int
	var releasedKey string
	var meteringConflict bool
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx.Model(&ArkPrefixCache{}).Scopes(arkCacheExecutionScope).Where("id = ? AND next_action_at > 0 AND next_action_at <= ?", id, now).
			UpdateColumn("next_action_at", now+60)
		if locked.Error != nil || locked.RowsAffected == 0 {
			return locked.Error
		}
		var cache ArkPrefixCache
		if err := lockForUpdate(tx).Take(&cache, "id = ?", id).Error; err != nil {
			return err
		}
		previousQuota := cache.SettledQuota
		previousHour := cache.NextHour
		end := arkCacheEnd(cache)
		if arkCacheUsesAutomaticStorage(cache) {
			if cache.State == ArkCacheInferencePending {
				// Inference recovery can price hours from the durable receipt even after the deadline.
				return nil
			}
			if cache.InferenceSettledAt > 0 && end <= cache.CreatedAt {
				// A confirmed delete at or before creation leaves no billable interval.
				end = min(end, cache.NextHour)
			}
		}
		if cache.FundsClosedAt == 0 && cache.MeterVersion != "" && cache.NextHour > 0 && cache.NextHour+3600 <= now && cache.NextHour < end &&
			(cache.State == ArkCacheActive || cache.State == ArkCacheDeleted || ((cache.State == ArkCacheDeleting || cache.State == ArkCacheAbsent) && now >= cache.ExpiresAt)) {
			if arkCacheUsesAutomaticStorage(cache) {
				if err := recordArkSessionStorageHours(tx, cache, now, end); err != nil {
					return err
				}
			}
			var hours []ArkPrefixCacheHour
			if err := tx.Where("cache_id = ? AND hour_start >= ? AND hour_start < ? AND settled_at = 0", id, cache.NextHour, end).Order("hour_start").Find(&hours).Error; err != nil {
				return err
			}
			if len(hours) > 0 && ShouldTrackBillingStatementRevision() {
				if err := lockBillingSourceUser(tx, cache.UserID); err != nil {
					return err
				}
			}
			for _, hour := range hours {
				if hour.HourStart != cache.NextHour || hour.HourStart+3600 > now {
					break
				}
				total := cache.SettledTokens + int64(hour.Tokens)
				quota, err := cache.storageQuota(total, false)
				if err != nil || quota < cache.SettledQuota || quota-cache.SettledQuota > cache.HeldQuota {
					meteringConflict, cache.MeterBlocked = true, true
					cache.NextActionAt = max(cache.NextActionAt, cache.FundsDeadline)
					break
				}
				delta := quota - cache.SettledQuota
				if err := tx.Model(&hour).Updates(map[string]any{"quota": delta, "settled_at": now}).Error; err != nil {
					return err
				}
				cache.SettledTokens, cache.SettledQuota, cache.NextHour = total, quota, hour.HourStart+3600
				cache.HeldQuota -= delta
			}
		}
		if arkCacheUsesAutomaticStorage(cache) && !meteringConflict && cache.NextHour > previousHour && cache.NextHour < end {
			cache.NextActionAt = cache.NextHour + 3600
		}
		if cache.FundsClosedAt == 0 && (cache.State == ArkCacheRejected || now >= cache.FundsDeadline ||
			(cache.NextHour > 0 && cache.NextHour >= end && now >= end)) {
			cache.FundsOutcome = "metered"
			if cache.State == ArkCacheRejected {
				cache.FundsOutcome = "rejected"
			} else if meteringConflict {
				cache.FundsOutcome = "meter_error"
			} else if cache.NextHour == 0 || cache.NextHour < end {
				cache.FundsOutcome = "deadline"
			}
			if cache.HeldQuota > 0 {
				var err error
				releasedKey, err = releaseArkPrefixCacheFunds(tx, cache)
				if err != nil {
					return err
				}
				releasedUser = cache.UserID
			}
			cache.ReleasedQuota = cache.HeldQuota
			cache.HeldQuota = 0
			cache.FundsClosedAt = now
			cache.NextActionAt = 0
		}
		if delta := cache.SettledQuota - previousQuota; delta > 0 {
			if err := tx.Model(&User{}).Where("id = ?", cache.UserID).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
				return err
			}
			if err := tx.Model(&Channel{}).Where("id = ?", cache.ChannelID).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
				return err
			}
		}
		return tx.Model(&cache).Updates(map[string]any{
			"held_quota": cache.HeldQuota, "settled_tokens": cache.SettledTokens, "settled_quota": cache.SettledQuota,
			"next_hour": cache.NextHour, "next_action_at": cache.NextActionAt, "funds_closed_at": cache.FundsClosedAt,
			"released_quota": cache.ReleasedQuota, "funds_outcome": cache.FundsOutcome, "meter_blocked": cache.MeterBlocked,
		}).Error
	})
	if err == nil && releasedUser > 0 {
		invalidateArkCacheBalances(releasedUser, releasedKey)
	}
	if err == nil && meteringConflict {
		return ErrArkCacheMeter
	}
	return err
}

func releaseArkPrefixCacheFunds(tx *gorm.DB, cache ArkPrefixCache) (string, error) {
	var token Token
	err := lockForUpdate(tx).Take(&token, "id = ? AND user_id = ?", cache.TokenID, cache.UserID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if token.Id != 0 {
		if err := tx.Model(&token).Updates(map[string]any{
			"remain_quota": gorm.Expr("remain_quota + ?", cache.HeldQuota),
			"used_quota":   gorm.Expr("used_quota - ?", cache.HeldQuota),
		}).Error; err != nil {
			return "", err
		}
	}
	result := tx.Model(&User{}).Where("id = ?", cache.UserID).UpdateColumn("quota", gorm.Expr("quota + ?", cache.HeldQuota))
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", ErrArkCacheFunds
	}
	return token.Key, nil
}

func ArkPrefixCacheLogsDue(ctx context.Context, now int64) (rows []ArkPrefixCacheHour, err error) {
	err = DB.WithContext(ctx).Scopes(arkCacheHourExecutionScope).
		Where("settled_at > 0 AND delivered_at = 0 AND (next_retry_at IS NULL OR next_retry_at <= ?)", now).
		Order("next_retry_at, settled_at, id").Limit(50).Find(&rows).Error
	return rows, err
}

func DeliverArkPrefixCacheLog(ctx context.Context, id, now int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx.Model(&ArkPrefixCacheHour{}).Scopes(arkCacheHourExecutionScope).
			Where("id = ? AND settled_at > 0 AND delivered_at = 0 AND (next_retry_at IS NULL OR next_retry_at <= ?)", id, now).
			UpdateColumn("delivered_at", now)
		if locked.Error != nil || locked.RowsAffected == 0 {
			return locked.Error
		}
		var hour ArkPrefixCacheHour
		if err := tx.Take(&hour, id).Error; err != nil {
			return err
		}
		var cache ArkPrefixCache
		if err := tx.Take(&cache, "id = ?", hour.CacheID).Error; err != nil {
			return err
		}
		var facts struct {
			Event        string            `json:"billing_event"`
			Tokens       int               `json:"storage_tokens"`
			Unit         string            `json:"storage_unit"`
			HourStart    int64             `json:"hour_start"`
			HourEnd      int64             `json:"hour_end"`
			UnitPrice    string            `json:"price_per_million_token_hour"`
			ContentParts []*common.Message `json:"content_parts"`
			Admin        struct {
				Quota        int    `json:"statement_original_quota"`
				MeterVersion string `json:"meter_version"`
				PriceVersion string `json:"storage_price_version,omitempty"`
				StorageTier  string `json:"storage_price_tier,omitempty"`
			} `json:"admin_info"`
		}
		facts.Event, facts.Tokens, facts.Unit = ArkCacheBillingMode, hour.Tokens, "token_hour"
		facts.HourStart, facts.HourEnd, facts.UnitPrice = hour.HourStart, hour.HourStart+3600, cache.PricePerMTokHr
		facts.ContentParts = []*common.Message{common.NewMessage("Cache storage fee")}
		facts.Admin.Quota, facts.Admin.MeterVersion = hour.Quota, cache.MeterVersion
		if cache.StorageExpr != "" {
			facts.Admin.PriceVersion = billingexpr.ExprHashString(cache.StorageExpr)
			facts.Admin.StorageTier = cache.StorageTier
		}
		encoded, err := common.Marshal(facts)
		if err != nil {
			return err
		}
		log := Log{Type: LogTypeConsume, RequestId: fmt.Sprintf("ark-storage:%d", hour.ID), CreatedAt: hour.HourStart,
			UserId: cache.UserID, Username: cache.Username, TokenId: cache.TokenID, TokenName: cache.TokenName,
			ModelName: cache.Model, ChannelId: cache.ChannelID, Group: cache.GroupName,
			Quota: hour.Quota, Other: string(encoded), Content: "Cache storage fee"}
		logDB := LOG_DB.WithContext(ctx)
		if LOG_DB == DB {
			logDB = tx
		}
		var prior Log
		err = logDB.Where("request_id = ?", log.RequestId).Take(&prior).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = logDB.Create(&log).Error
		} else if err == nil && (prior.Type != log.Type || prior.CreatedAt != log.CreatedAt || prior.ChannelId != log.ChannelId ||
			prior.UserId != log.UserId || prior.TokenId != log.TokenId || prior.ModelName != log.ModelName || prior.Quota != log.Quota || prior.Other != log.Other) {
			err = ErrArkCacheConflict
		}
		if err != nil {
			return err
		}
		if common.DataExportEnabled {
			return tx.Create(&QuotaData{UserID: log.UserId, Username: log.Username, ModelName: log.ModelName,
				CreatedAt: hour.HourStart, UseGroup: log.Group, TokenID: log.TokenId, ChannelID: log.ChannelId,
				NodeName: common.NodeName, Quota: log.Quota}).Error
		}
		return nil
	})
}
