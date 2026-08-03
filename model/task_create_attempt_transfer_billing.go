package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type taskAttemptTransferBillingResult struct {
	UserID      int
	WalletDelta int
	TokenKey    string
	TokenDelta  int
}

// settleTaskCreateAttemptTransferTx makes the amount owned by the durable Task
// equal to its final submit-time quota before the attempt hold is transferred.
// A crash can therefore leave either the attempt hold or the Task amount, never
// a Task whose persisted quota disagrees with the funds actually reserved.
func settleTaskCreateAttemptTransferTx(
	tx *gorm.DB,
	attempt *TaskCreateAttempt,
	task *Task,
) (taskAttemptTransferBillingResult, error) {
	result := taskAttemptTransferBillingResult{}
	if tx == nil || attempt == nil || task == nil || task.Quota < 0 {
		return result, errors.New("task attempt transfer billing is incomplete")
	}
	if attempt.Status != TaskCreateAttemptUpstreamSucceeded ||
		attempt.BillingHoldState != TaskCreateAttemptBillingHeld {
		return result, errors.New("task create attempt has no transferable hold")
	}

	result.UserID = attempt.UserID
	fundingDelta := task.Quota - attempt.HeldQuota
	if fundingDelta != 0 {
		switch attempt.BillingSource {
		case "subscription":
			var subscription UserSubscription
			if err := lockForUpdate(tx).Where("id = ?", attempt.SubscriptionID).First(&subscription).Error; err != nil {
				return result, err
			}
			used := subscription.AmountUsed + int64(fundingDelta)
			if used < 0 {
				return result, fmt.Errorf("subscription used would become negative: %d", used)
			}
			if subscription.AmountTotal > 0 && used > subscription.AmountTotal {
				return result, fmt.Errorf(
					"subscription used exceeds total, used=%d total=%d",
					used,
					subscription.AmountTotal,
				)
			}
			if err := tx.Model(&subscription).Update("amount_used", used).Error; err != nil {
				return result, err
			}
		case "wallet":
			if fundingDelta > 0 {
				update := tx.Model(&User{}).
					Where("id = ? AND quota >= ?", attempt.UserID, fundingDelta).
					Update("quota", gorm.Expr("quota - ?", fundingDelta))
				if update.Error != nil {
					return result, update.Error
				}
				if update.RowsAffected != 1 {
					return result, fmt.Errorf("insufficient quota for task transfer delta %d", fundingDelta)
				}
			} else {
				update := tx.Model(&User{}).Where("id = ?", attempt.UserID).
					Update("quota", gorm.Expr("quota + ?", -fundingDelta))
				if update.Error != nil {
					return result, update.Error
				}
				if update.RowsAffected != 1 {
					return result, fmt.Errorf("task transfer user %d not found", attempt.UserID)
				}
			}
			result.WalletDelta = fundingDelta
		default:
			return result, fmt.Errorf("unsupported task attempt billing source %q", attempt.BillingSource)
		}
	}

	tokenTracked := attempt.TokenQuotaTracked || attempt.TokenQuotaHeld
	currentTokenQuota := 0
	if attempt.TokenQuotaHeld {
		currentTokenQuota = attempt.HeldQuota
	}
	targetTokenQuota := 0
	if tokenTracked {
		targetTokenQuota = task.Quota
	}
	tokenDelta := targetTokenQuota - currentTokenQuota
	if tokenDelta != 0 && attempt.TokenID <= 0 {
		return result, errors.New("task attempt token accounting target is missing")
	}
	if tokenDelta != 0 && attempt.TokenID > 0 {
		var token Token
		tokenResult := lockForUpdate(tx).Unscoped().Where("id = ?", attempt.TokenID).First(&token)
		if tokenResult.Error != nil && tokenResult.Error != gorm.ErrRecordNotFound {
			return result, tokenResult.Error
		}
		if tokenResult.Error == nil && !token.DeletedAt.Valid {
			updates := map[string]any{"accessed_time": common.GetTimestamp()}
			query := tx.Model(&token).Where("id = ?", token.Id)
			if tokenDelta > 0 {
				query = query.Where("unlimited_quota = ? OR remain_quota >= ?", true, tokenDelta)
				updates["remain_quota"] = gorm.Expr("remain_quota - ?", tokenDelta)
				updates["used_quota"] = gorm.Expr("used_quota + ?", tokenDelta)
			} else {
				updates["remain_quota"] = gorm.Expr("remain_quota + ?", -tokenDelta)
				updates["used_quota"] = gorm.Expr("used_quota - ?", -tokenDelta)
			}
			update := query.Updates(updates)
			if update.Error != nil {
				return result, update.Error
			}
			if update.RowsAffected != 1 {
				return result, fmt.Errorf("insufficient token quota for task transfer delta %d", tokenDelta)
			}
			result.TokenKey = token.Key
			result.TokenDelta = tokenDelta
		}
	}

	if attempt.BillingSource == "subscription" && attempt.SubscriptionID > 0 {
		update := tx.Model(&SubscriptionPreConsumeRecord{}).
			Where("request_id = ? AND status = ?", attempt.AttemptID, "consumed").
			Updates(map[string]any{
				"pre_consumed": int64(task.Quota),
				"status":       "transferred",
				"updated_at":   common.GetTimestamp(),
			})
		if update.Error != nil {
			return result, update.Error
		}
		if update.RowsAffected != 1 {
			return result, errors.New("task attempt subscription hold record was not transferred")
		}
	}

	task.PrivateData.BillingSource = attempt.BillingSource
	task.PrivateData.SubscriptionId = attempt.SubscriptionID
	task.PrivateData.TokenId = attempt.TokenID
	task.PrivateData.SkipTokenQuota = !tokenTracked
	if task.PrivateData.AsyncBilling != nil {
		task.PrivateData.AsyncBilling.State = TaskBillingStatePending
		task.PrivateData.AsyncBilling.Error = ""
		task.PrivateData.AsyncBilling.NextRetryAt = 0
	}
	task.BillingState = deriveBillingState(task.PrivateData)
	return result, nil
}

func syncTaskCreateAttemptTransferCache(result taskAttemptTransferBillingResult) {
	if result.WalletDelta != 0 {
		gopool.Go(func() {
			var err error
			if result.WalletDelta > 0 {
				err = cacheDecrUserQuota(result.UserID, int64(result.WalletDelta))
			} else {
				err = cacheIncrUserQuota(result.UserID, int64(-result.WalletDelta))
			}
			if err != nil {
				common.SysError("failed to update task transfer wallet cache: " + err.Error())
			}
		})
	}
	if result.TokenDelta != 0 && result.TokenKey != "" && common.RedisEnabled && common.RDB != nil {
		gopool.Go(func() {
			var err error
			if result.TokenDelta > 0 {
				err = cacheDecrTokenQuota(result.TokenKey, int64(result.TokenDelta))
			} else {
				err = cacheIncrTokenQuota(result.TokenKey, int64(-result.TokenDelta))
			}
			if err != nil {
				common.SysError("failed to update task transfer token cache: " + err.Error())
			}
		})
	}
}
