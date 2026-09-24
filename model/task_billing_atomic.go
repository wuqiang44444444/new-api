package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// ErrTaskBillingInsufficientFunding identifies a retryable settlement shortfall.
// Callers use it to distinguish customer funding debt from infrastructure errors
// without parsing provider- or database-specific error text.
var ErrTaskBillingInsufficientFunding = errors.New("insufficient task billing funding")

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
	requestedClamp := (*common.QuotaClamp)(nil)
	requestedCalculation := task.PrivateData.AsyncBilling
	requestedContext := task.PrivateData.BillingContext
	if task.PrivateData.AsyncBilling != nil {
		requestedClamp = task.PrivateData.AsyncBilling.QuotaClamp
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
		if err := lockImageTaskBillingTx(tx, task); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.VideoRefundState != "" {
			return nil
		}
		async := locked.PrivateData.AsyncBilling
		nativeTask := async == nil
		// Native usage settlement has always allowed wallet/token overdrafts.
		// Keep the existing funding guard for durable async and legacy video tasks.
		requireAvailableQuota := !nativeTask || IsVideoFundTask(&locked)
		if nativeTask {
			async = &TaskAsyncBillingContext{}
		}
		if async.State == TaskBillingStateSettled {
			return nil
		}
		if err := enforceSeedanceBillingTarget(&locked, targetQuota); err != nil {
			return err
		}
		if async.TargetQuota != nil && *async.TargetQuota != targetQuota {
			return fmt.Errorf("task billing target already accepted")
		}
		if requestedCalculation != nil && async.TargetQuota == nil {
			async.CalculationVersion, async.Calculation, async.CalculationSource = requestedCalculation.CalculationVersion, requestedCalculation.Calculation, requestedCalculation.CalculationSource
		}
		if nativeTask && requestedContext != nil {
			if locked.PrivateData.BillingContext == nil {
				locked.PrivateData.BillingContext = &TaskBillingContext{}
			}
			if requestedContext.SettlementCalculationVersion > 0 && (requestedContext.SettlementCalculation == nil || requestedContext.SettlementCalculation.Quota != targetQuota) {
				return fmt.Errorf("billing calculation missing or inconsistent")
			}
			if locked.PrivateData.BillingContext.SettlementCalculation != nil {
				return nil
			}
			locked.PrivateData.BillingContext.SettlementCalculation = requestedContext.SettlementCalculation
			if locked.PrivateData.BillingContext.TieredSnapshot != nil && requestedContext.TieredSnapshot != nil {
				locked.PrivateData.BillingContext.TieredSnapshot.UsageFacts = requestedContext.TieredSnapshot.UsageFacts
				locked.PrivateData.BillingContext.TieredSnapshot.EstimatedTier = requestedContext.TieredSnapshot.EstimatedTier
			}
			locked.PrivateData.BillingContext.SettlementCalculationVersion = requestedContext.SettlementCalculationVersion
		}
		if err := validateTaskCalculation(&locked, async, targetQuota); err != nil {
			return err
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
		if requestedClamp != nil {
			async.QuotaClamp = requestedClamp
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
					return fmt.Errorf("%w: subscription used exceeds total, used=%d total=%d", ErrTaskBillingInsufficientFunding, used, subscription.AmountTotal)
				}
				if err := tx.Model(&subscription).Update("amount_used", used).Error; err != nil {
					return err
				}
			} else if delta > 0 {
				query := tx.Model(&User{}).Where("id = ?", locked.UserId)
				if requireAvailableQuota {
					query = query.Where("quota >= ?", delta)
				}
				result := query.Update("quota", gorm.Expr("quota - ?", delta))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					var user User
					if err := tx.Select("id").Where("id = ?", locked.UserId).First(&user).Error; err != nil {
						return err
					}
					return fmt.Errorf("%w: insufficient wallet quota for task billing delta %d", ErrTaskBillingInsufficientFunding, delta)
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
						if requireAvailableQuota {
							query = query.Where("unlimited_quota = ? OR remain_quota >= ?", true, delta)
						}
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
						return fmt.Errorf("%w: insufficient token quota for task billing delta %d", ErrTaskBillingInsufficientFunding, delta)
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

		event := "adjustment"
		if async.Operation == "refund" {
			event = "refund"
		}
		if IsImageTask(&locked) {
			event = "complete"
		}
		if err := queueTaskBillingDeliveryTx(tx, &locked, event, locked.Quota, targetQuota); err != nil {
			return err
		}
		locked.Quota = targetQuota
		async.State = TaskBillingStateSettled
		async.Error = ""
		async.NextRetryAt = 0
		if nativeTask {
			if err := tx.Model(&locked).Updates(map[string]any{"quota": targetQuota, "private_data": locked.PrivateData}).Error; err != nil {
				return err
			}
			applied = true
			return nil
		}
		if err := tx.Model(&locked).Updates(map[string]any{
			"quota":         targetQuota,
			"private_data":  locked.PrivateData,
			"billing_state": TaskBillingStateSettled,
		}).Error; err != nil {
			return err
		}
		if err := completeImageTaskBillingTx(tx, &locked); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	task.Quota = locked.Quota
	task.PrivateData = locked.PrivateData
	task.BillingState = locked.BillingState
	if !applied {
		return false, 0, nil
	}

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
