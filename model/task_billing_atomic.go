package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// ApplyTaskBillingTarget atomically adjusts the wallet/subscription funding
// source and advances the task billing state. Locking and the settled-state
// check make retries idempotent even after the task itself is already terminal.
func ApplyTaskBillingTarget(task *Task, targetQuota int) (bool, int, error) {
	return applyTaskBillingTarget(task, targetQuota, nil)
}

func applyTaskBillingTarget(task *Task, targetQuota int, exposure *ProviderCostExposure) (bool, int, error) {
	if task == nil || task.ID == 0 || targetQuota < 0 {
		return false, 0, fmt.Errorf("invalid task billing target")
	}

	var locked Task
	requestedOperation := ""
	requestedReason := ""
	requestedTargetQuota := (*int)(nil)
	if task.PrivateData.AsyncBilling != nil {
		requestedOperation = task.PrivateData.AsyncBilling.Operation
		requestedReason = task.PrivateData.AsyncBilling.Reason
		if task.PrivateData.AsyncBilling.TargetQuota != nil {
			target := *task.PrivateData.AsyncBilling.TargetQuota
			requestedTargetQuota = &target
		}
	}
	tokenKey := ""
	delta := 0
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&locked).Error; err != nil {
			return err
		}
		async := locked.PrivateData.AsyncBilling
		if async == nil {
			return fmt.Errorf("task %s has no async billing state", locked.TaskID)
		}
		if async.State == TaskBillingStateSettled {
			return nil
		}
		if requestedOperation != "" {
			async.Operation = requestedOperation
		}
		if requestedReason != "" {
			async.Reason = requestedReason
		}
		if requestedTargetQuota != nil {
			target := *requestedTargetQuota
			async.TargetQuota = &target
		}

		delta = targetQuota - locked.Quota
		if delta != 0 {
			if locked.PrivateData.BillingSource == "subscription" && locked.PrivateData.SubscriptionId > 0 {
				var subscription UserSubscription
				if err := lockForUpdate(tx).Where("id = ?", locked.PrivateData.SubscriptionId).First(&subscription).Error; err != nil {
					return err
				}
				used := subscription.AmountUsed + int64(delta)
				if used < 0 {
					used = 0
				}
				if subscription.AmountTotal > 0 && used > subscription.AmountTotal {
					return fmt.Errorf("subscription used exceeds total, used=%d total=%d", used, subscription.AmountTotal)
				}
				if err := tx.Model(&subscription).Update("amount_used", used).Error; err != nil {
					return err
				}
			} else if delta > 0 {
				result := tx.Model(&User{}).
					Where("id = ? AND quota >= ?", locked.UserId, delta).
					Update("quota", gorm.Expr("quota - ?", delta))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("insufficient quota for task billing delta %d", delta)
				}
			} else {
				result := tx.Model(&User{}).Where("id = ?", locked.UserId).
					Update("quota", gorm.Expr("quota + ?", -delta))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("task billing user %d not found", locked.UserId)
				}
			}

			if locked.PrivateData.TokenId > 0 && !locked.PrivateData.SkipTokenQuota {
				var token Token
				tokenResult := tx.Unscoped().Where("id = ?", locked.PrivateData.TokenId).First(&token)
				if tokenResult.Error != nil && tokenResult.Error != gorm.ErrRecordNotFound {
					return tokenResult.Error
				}
				if tokenResult.Error == nil && !token.DeletedAt.Valid {
					tokenKey = token.Key
					updates := map[string]any{
						"accessed_time": common.GetTimestamp(),
					}
					query := tx.Model(&token).Where("id = ?", token.Id)
					if delta > 0 {
						query = query.Where("unlimited_quota = ? OR remain_quota >= ?", true, delta)
						updates["remain_quota"] = gorm.Expr("remain_quota - ?", delta)
						updates["used_quota"] = gorm.Expr("used_quota + ?", delta)
					} else {
						updates["remain_quota"] = gorm.Expr("remain_quota + ?", -delta)
						updates["used_quota"] = gorm.Expr("used_quota - ?", -delta)
					}
					result := query.Updates(updates)
					if result.Error != nil {
						return result.Error
					}
					if result.RowsAffected != 1 {
						return fmt.Errorf("insufficient token quota for task billing delta %d", delta)
					}
				}
			}
		}

		if exposure != nil {
			if exposure.CustomerQuotaReleased == 0 && delta < 0 {
				exposure.CustomerQuotaReleased = -delta
			}
			if err := insertProviderCostExposureTx(tx, exposure); err != nil {
				return err
			}
		}

		locked.Quota = targetQuota
		async.State = TaskBillingStateSettled
		async.Error = ""
		async.NextRetryAt = 0
		if err := tx.Model(&locked).Updates(map[string]any{
			"quota":         targetQuota,
			"private_data":  locked.PrivateData,
			"billing_state": TaskBillingStateSettled,
		}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	if !applied {
		return false, 0, nil
	}

	task.Quota = locked.Quota
	task.PrivateData = locked.PrivateData
	if delta != 0 && !(locked.PrivateData.BillingSource == "subscription" && locked.PrivateData.SubscriptionId > 0) {
		gopool.Go(func() {
			var cacheErr error
			if delta > 0 {
				cacheErr = cacheDecrUserQuota(locked.UserId, int64(delta))
			} else {
				cacheErr = cacheIncrUserQuota(locked.UserId, int64(-delta))
			}
			if cacheErr != nil {
				common.SysError("failed to update user quota cache after task billing: " + cacheErr.Error())
			}
		})
	}
	if delta != 0 && tokenKey != "" && common.RedisEnabled {
		gopool.Go(func() {
			var cacheErr error
			if delta > 0 {
				cacheErr = cacheDecrTokenQuota(tokenKey, int64(delta))
			} else {
				cacheErr = cacheIncrTokenQuota(tokenKey, int64(-delta))
			}
			if cacheErr != nil {
				common.SysError("failed to update token quota cache after task billing: " + cacheErr.Error())
			}
		})
	}
	return true, delta, nil
}
