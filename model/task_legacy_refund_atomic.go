package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// RefundLegacyTaskQuota atomically refunds a task that predates AsyncBilling.
// The task quota is the idempotency marker: it is cleared in the same
// transaction as wallet/subscription and token adjustments.
func RefundLegacyTaskQuota(task *Task) (bool, int, error) {
	if task == nil || task.ID == 0 {
		return false, 0, fmt.Errorf("invalid task refund")
	}

	var locked Task
	refundedQuota := 0
	tokenKey := ""
	subscriptionFunding := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Quota == 0 {
			return nil
		}
		refundedQuota = locked.Quota
		subscriptionFunding = locked.PrivateData.BillingSource == "subscription" &&
			locked.PrivateData.SubscriptionId > 0

		if subscriptionFunding {
			var subscription UserSubscription
			if err := lockForUpdate(tx).
				Where("id = ?", locked.PrivateData.SubscriptionId).
				First(&subscription).Error; err != nil {
				return err
			}
			used := subscription.AmountUsed - int64(refundedQuota)
			if used < 0 {
				used = 0
			}
			if err := tx.Model(&subscription).Update("amount_used", used).Error; err != nil {
				return err
			}
		} else {
			result := tx.Model(&User{}).Where("id = ?", locked.UserId).
				Update("quota", gorm.Expr("quota + ?", refundedQuota))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("task refund user %d not found", locked.UserId)
			}
		}

		if locked.PrivateData.TokenId > 0 && !locked.PrivateData.SkipTokenQuota {
			var token Token
			result := tx.Unscoped().Where("id = ?", locked.PrivateData.TokenId).First(&token)
			if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
				return result.Error
			}
			if result.Error == nil && !token.DeletedAt.Valid {
				tokenKey = token.Key
				if err := tx.Model(&token).Updates(map[string]any{
					"remain_quota":  gorm.Expr("remain_quota + ?", refundedQuota),
					"used_quota":    gorm.Expr("used_quota - ?", refundedQuota),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
			}
		}

		if err := tx.Model(&locked).Update("quota", 0).Error; err != nil {
			return err
		}
		locked.Quota = 0
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	task.Quota = locked.Quota
	if refundedQuota == 0 {
		return false, 0, nil
	}

	if !subscriptionFunding {
		gopool.Go(func() {
			if err := cacheIncrUserQuota(locked.UserId, int64(refundedQuota)); err != nil {
				common.SysError("failed to update user quota cache after task refund: " + err.Error())
			}
		})
	}
	if tokenKey != "" && common.RedisEnabled {
		gopool.Go(func() {
			if err := cacheIncrTokenQuota(tokenKey, int64(refundedQuota)); err != nil {
				common.SysError("failed to update token quota cache after task refund: " + err.Error())
			}
		})
	}
	return true, refundedQuota, nil
}
