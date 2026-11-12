package model

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type quotaDataFlushReceipt struct {
	ID string `gorm:"size:32;primaryKey"`
}

type quotaDataFlushChunk struct {
	id   string
	rows []*QuotaData
}

var quotaDataFlushLock sync.Mutex
var pendingQuotaDataFlush []quotaDataFlushChunk

func SaveQuotaDataCache() {
	quotaDataFlushLock.Lock()
	defer quotaDataFlushLock.Unlock()

	if len(pendingQuotaDataFlush) == 0 {
		CacheQuotaDataLock.Lock()
		cache := CacheQuotaData
		CacheQuotaData = make(map[string]*QuotaData)
		CacheQuotaDataLock.Unlock()

		keys := make([]string, 0, len(cache))
		for key := range cache {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for start := 0; start < len(keys); start += 50 {
			chunk := quotaDataFlushChunk{id: common.GetUUID()}
			for _, key := range keys[start:min(start+50, len(keys))] {
				chunk.rows = append(chunk.rows, cache[key])
			}
			pendingQuotaDataFlush = append(pendingQuotaDataFlush, chunk)
		}
	}

	for len(pendingQuotaDataFlush) > 0 {
		chunk := pendingQuotaDataFlush[0]
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := persistQuotaDataFlushChunk(ctx, DB, chunk)
		cancel()
		if err != nil {
			common.SysError("dashboard usage flush failed: " + err.Error())
			return
		}
		pendingQuotaDataFlush[0] = quotaDataFlushChunk{}
		pendingQuotaDataFlush = pendingQuotaDataFlush[1:]
		// Once the commit is confirmed, this process no longer retries the chunk.
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		err = DB.WithContext(ctx).Delete(&quotaDataFlushReceipt{ID: chunk.id}).Error
		cancel()
		if err != nil {
			common.SysError("dashboard usage flush receipt cleanup failed: " + err.Error())
		}
	}
}

func persistQuotaDataFlushChunk(ctx context.Context, db *gorm.DB, chunk quotaDataFlushChunk) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var receipt quotaDataFlushReceipt
		err := tx.Take(&receipt, "id = ?", chunk.id).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&quotaDataFlushReceipt{ID: chunk.id}).Error; err != nil {
			return err
		}
		for _, row := range chunk.rows {
			var existing QuotaData
			err := tx.Select("id").
				Where("user_id = ? AND username = ? AND model_name = ? AND created_at = ? AND use_group = ? AND token_id = ? AND channel_id = ? AND node_name = ?",
					row.UserID, row.Username, row.ModelName, row.CreatedAt, row.UseGroup, row.TokenID, row.ChannelID, row.NodeName).
				First(&existing).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				copy := *row
				if err := tx.Create(&copy).Error; err != nil {
					return err
				}
			case err != nil:
				return err
			default:
				if err := tx.Model(&existing).Updates(map[string]any{
					"count":      gorm.Expr("count + ?", row.Count),
					"quota":      gorm.Expr("quota + ?", row.Quota),
					"token_used": gorm.Expr("token_used + ?", row.TokenUsed),
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
