package model

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ArkSessionLocalMeterVersion bills each round's new tokens for each hour
// it touched. It is a local test tariff and makes no supplier billing claim.
const ArkSessionLocalMeterVersion = "session-local-increment-v1"

var arkSessionLocalBillingAllowed = setting.ArkCacheIsolatedLocalDatabase

// Pending test rows must not consume funds or take recovery slots outside
// the isolated database, even when restored from a snapshot.
func arkCacheExecutionScope(query *gorm.DB) *gorm.DB {
	if !arkSessionLocalBillingAllowed() {
		return query.Where("(ark_prefix_caches.meter_version IS NULL OR ark_prefix_caches.meter_version <> ?)", ArkSessionLocalMeterVersion)
	}
	return query
}

func arkCacheHourExecutionScope(query *gorm.DB) *gorm.DB {
	if !arkSessionLocalBillingAllowed() {
		resources := query.Session(&gorm.Session{NewDB: true}).Model(&ArkPrefixCache{}).Select("id").Scopes(arkCacheExecutionScope)
		return query.Where("cache_id IN (?)", resources)
	}
	return query
}

func (row *ArkPrefixCache) ReserveArkSessionLocalStorage(price string, maxTokens int) error {
	row.PricePerMTokHr = price
	row.QuotaPerUnit = strconv.FormatFloat(common.QuotaPerUnit, 'g', -1, 64)
	row.MaxStored = maxTokens
	// A 30-minute session (including accepted timestamp skew) can touch two hours.
	held, err := ArkPrefixCacheQuota(price, row.QuotaPerUnit, int64(maxTokens)*2, true)
	if err != nil {
		return err
	}
	row.HeldQuota = held
	row.FundsDeadline = (row.ExpiresAt-1)/3600*3600 + 2*3600
	row.NextActionAt = row.FundsDeadline
	return nil
}

func arkSessionStorageTokens(row ArkPrefixCache) int {
	return row.InputTokens - row.ReadTokens + row.OutputTokens
}

func arkSessionLocalStorageAfterInference(row ArkPrefixCache) (int, int64, error) {
	firstHour := row.CreatedAt / 3600 * 3600
	hours := (row.ExpiresAt-1-firstHour)/3600 + 1
	tokens := arkSessionStorageTokens(row)
	if row.CreatedAt <= 0 || row.ExpiresAt <= row.CreatedAt || hours > 2 || tokens < 0 || tokens > row.MaxStored {
		return 0, 0, ErrArkCacheMeter
	}
	held, err := ArkPrefixCacheQuota(row.PricePerMTokHr, row.QuotaPerUnit, int64(tokens)*hours, true)
	if err != nil || held > row.HeldQuota {
		return 0, 0, ErrArkCacheMeter
	}
	return held, firstHour, nil
}

// The settlement transaction owns the resource row. It records the derived
// hour and consumes the corresponding hold together.
func recordArkSessionLocalHours(tx *gorm.DB, row ArkPrefixCache, now, end int64) error {
	if row.SessionRound == 0 || row.InferenceSettledAt == 0 || row.MeterBlocked {
		return nil
	}
	tokens := arkSessionStorageTokens(row)
	for hour := row.NextHour; hour < end && hour+3600 <= now; hour += 3600 {
		if (row.State == ArkCacheDeleting || row.State == ArkCacheAbsent) &&
			(row.DeleteRequestedAt == 0 || row.DeleteRequestedAt < min(end, hour+3600)) {
			break
		}
		evidence := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", ArkSessionLocalMeterVersion, row.ID, hour)))
		record := ArkPrefixCacheHour{CacheID: row.ID, HourStart: hour, Tokens: tokens, Evidence: fmt.Sprintf("%x", evidence)}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "cache_id"}, {Name: "hour_start"}}, DoNothing: true,
		}).Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}
