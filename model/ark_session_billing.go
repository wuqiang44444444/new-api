package model

import (
	"context"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// SettleArkSessionInference applies the frozen inference charge once. The same
// transaction activates the response and makes its consumption event deliverable.
func SettleArkSessionInference(ctx context.Context, id string, now int64) error {
	var tokenKey string
	var userID int
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ArkPrefixCache{}).Scopes(arkCacheExecutionScope).
			Where("id = ? AND inference_next_at > 0 AND inference_next_at <= ?", id, now).
			UpdateColumn("inference_next_at", now+60)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		var row ArkPrefixCache
		if err := lockForUpdate(tx).Take(&row, "id = ?", id).Error; err != nil {
			return err
		}
		fields := map[string]any{"inference_next_at": 0, "inference_held_quota": 0}
		refund := row.InferenceHeldQuota
		if row.State == ArkCacheInferencePending {
			if row.InferenceQuota < 0 || row.InferenceQuota > row.InferenceHeldQuota || row.InferenceLog == "" {
				return ErrArkCacheConflict
			}
			refund -= row.InferenceQuota
			fields["state"] = ArkCacheActive
			fields["inference_settled_at"] = now
			if arkCacheUsesAutomaticStorage(row) && row.FundsClosedAt == 0 {
				held, firstHour, err := arkSessionStorageAfterInference(row)
				if err != nil {
					return err
				}
				refund += row.HeldQuota - held
				fields["held_quota"], fields["next_hour"], fields["next_action_at"] = held, firstHour, firstHour+3600
			}
			if common.LogConsumeEnabled {
				if ShouldTrackBillingStatementRevision() {
					if err := lockBillingSourceUser(tx, row.UserID); err != nil {
						return err
					}
				}
				fields["inference_log_due_at"] = now
			} else {
				fields["inference_log"] = ""
			}
		} else if row.State == ArkCacheSending {
			fields["state"] = ArkCacheUnknown
		} else if row.State == ArkCacheUnknown && row.ProviderID != "" {
			// The caller never received a valid result. Retire the orphan using
			// the known ID while preserving its unknown inference outcome.
			fields["state"] = ArkCacheDeleting
			fields["delete_requested_at"] = now
			fields["delete_check_at"] = now
		}
		if refund > 0 {
			var token Token
			err := lockForUpdate(tx).Take(&token, "id = ? AND user_id = ?", row.TokenID, row.UserID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if token.Id != 0 {
				if err := tx.Model(&token).Updates(map[string]any{
					"remain_quota": gorm.Expr("remain_quota + ?", refund),
					"used_quota":   gorm.Expr("used_quota - ?", refund),
				}).Error; err != nil {
					return err
				}
				tokenKey = token.Key
			}
			wallet := tx.Model(&User{}).Where("id = ?", row.UserID).UpdateColumn("quota", gorm.Expr("quota + ?", refund))
			if wallet.Error != nil {
				return wallet.Error
			}
			if wallet.RowsAffected != 1 {
				return ErrArkCacheFunds
			}
			userID = row.UserID
		}
		if row.State == ArkCacheInferencePending {
			if err := tx.Model(&User{}).Where("id = ?", row.UserID).Updates(map[string]any{
				"used_quota":    gorm.Expr("used_quota + ?", row.InferenceQuota),
				"request_count": gorm.Expr("request_count + ?", 1),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Channel{}).Where("id = ?", row.ChannelID).
				UpdateColumn("used_quota", gorm.Expr("used_quota + ?", row.InferenceQuota)).Error; err != nil {
				return err
			}
		}
		return tx.Model(&row).Updates(fields).Error
	})
	if err == nil && userID != 0 {
		invalidateArkCacheBalances(userID, tokenKey)
	}
	return err
}

func ArkSessionInferenceDue(ctx context.Context, now int64) (rows []ArkPrefixCache, err error) {
	err = DB.WithContext(ctx).Select("id").Scopes(arkCacheExecutionScope).
		Where("inference_next_at > 0 AND inference_next_at <= ?", now).
		Order("inference_next_at, id").Limit(50).Find(&rows).Error
	return rows, err
}

func ArkSessionLogsDue(ctx context.Context, now int64) (rows []ArkPrefixCache, err error) {
	err = DB.WithContext(ctx).Select("id").Scopes(arkCacheExecutionScope).
		Where("inference_log_due_at > 0 AND inference_log_due_at <= ?", now).
		Order("inference_log_due_at, id").Limit(50).Find(&rows).Error
	return rows, err
}

// DeliverArkSessionLog clears the staged projection after delivery. A split
// log database can commit before the main database does, so retries match the
// original event before updating dashboard data.
func DeliverArkSessionLog(ctx context.Context, id string) error {
	now := common.GetTimestamp()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx.Model(&ArkPrefixCache{}).Scopes(arkCacheExecutionScope).Where("id = ? AND inference_log_due_at > 0 AND inference_log_due_at <= ?", id, now).
			UpdateColumn("inference_log_due_at", now+60)
		if locked.Error != nil || locked.RowsAffected == 0 {
			return locked.Error
		}
		var row ArkPrefixCache
		if err := lockForUpdate(tx).Take(&row, "id = ?", id).Error; err != nil {
			return err
		}
		log := Log{Type: LogTypeConsume, RequestId: "ark-session:" + strings.TrimPrefix(id, ArkSessionCacheIDPrefix),
			CreatedAt: row.CreatedAt, UserId: row.UserID, Username: row.Username, TokenId: row.TokenID, TokenName: row.TokenName,
			ModelName: row.Model, ChannelId: row.ChannelID, Group: row.GroupName, Quota: row.InferenceQuota,
			PromptTokens: row.InputTokens, CompletionTokens: row.OutputTokens, Other: row.InferenceLog}
		logDB := LOG_DB.WithContext(ctx)
		if LOG_DB == DB {
			logDB = tx
		}
		var prior Log
		err := logDB.Where("request_id = ?", log.RequestId).Take(&prior).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = logDB.Create(&log).Error
		} else if err == nil && (prior.Type != log.Type || prior.CreatedAt != log.CreatedAt ||
			prior.UserId != log.UserId || prior.TokenId != log.TokenId || prior.ChannelId != log.ChannelId ||
			prior.Username != log.Username || prior.TokenName != log.TokenName || prior.ModelName != log.ModelName || prior.Group != log.Group ||
			prior.Quota != log.Quota || prior.PromptTokens != log.PromptTokens || prior.CompletionTokens != log.CompletionTokens || prior.Other != log.Other) {
			err = ErrArkCacheConflict
		}
		if err != nil {
			return err
		}
		if common.DataExportEnabled {
			if err := tx.Create(&QuotaData{UserID: row.UserID, Username: row.Username, ModelName: row.Model,
				CreatedAt: row.CreatedAt - row.CreatedAt%3600, UseGroup: row.GroupName, TokenID: row.TokenID, ChannelID: row.ChannelID,
				NodeName: common.NodeName, Count: 1, Quota: row.InferenceQuota, TokenUsed: row.InputTokens + row.OutputTokens,
				CacheReadTokens: int64(row.ReadTokens)}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&row).Updates(map[string]any{"inference_log": "", "inference_log_due_at": 0}).Error
	})
}
