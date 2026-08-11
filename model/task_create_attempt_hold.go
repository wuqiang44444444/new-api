package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

var (
	ErrTaskAttemptInsufficientQuota       = errors.New("task attempt quota is insufficient")
	ErrTaskAttemptSubscriptionUnavailable = errors.New("task attempt subscription is unavailable")
)

type TaskAttemptHoldParams struct {
	AttemptID     int64
	FundingSource string
	ModelName     string
	Quota         int
	IsPlayground  bool
}

type TaskAttemptHoldResult struct {
	BillingSource           string
	HeldQuota               int
	SubscriptionID          int
	SubscriptionPreConsumed int64
	SubscriptionAmountTotal int64
	SubscriptionAmountUsed  int64
	SubscriptionPlanID      int
	SubscriptionPlanTitle   string
	TokenKey                string
	TokenTracked            bool
	TokenDebited            bool
}

// HoldTaskCreateAttempt atomically reserves quota and the durable attempt
// state before the first outbound POST.
func HoldTaskCreateAttempt(params TaskAttemptHoldParams) (*TaskAttemptHoldResult, error) {
	if params.AttemptID <= 0 || params.Quota < 0 {
		return nil, errors.New("invalid task attempt hold")
	}
	source := strings.TrimSpace(params.FundingSource)
	if source != "wallet" && source != "subscription" {
		return nil, errors.New("invalid task attempt funding source")
	}
	result := &TaskAttemptHoldResult{BillingSource: source}
	var userID int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", params.AttemptID).Error; err != nil {
			return err
		}
		if attempt.Status != TaskCreateAttemptPrepared || attempt.BillingHoldState != TaskCreateAttemptBillingUnheld {
			return errors.New("task attempt is not prepared")
		}
		userID = attempt.UserID
		heldQuota := params.Quota
		if source == "subscription" && heldQuota == 0 {
			heldQuota = 1
		}
		result.HeldQuota = heldQuota
		result.TokenTracked = !params.IsPlayground
		if heldQuota > 0 {
			if !params.IsPlayground {
				var token Token
				if err := lockForUpdate(tx).First(&token, "id = ? AND user_id = ?", attempt.TokenID, attempt.UserID).Error; err != nil {
					return err
				}
				if !token.UnlimitedQuota && token.RemainQuota < heldQuota {
					return fmt.Errorf("%w: token", ErrTaskAttemptInsufficientQuota)
				}
				if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
					"remain_quota":  gorm.Expr("remain_quota - ?", heldQuota),
					"used_quota":    gorm.Expr("used_quota + ?", heldQuota),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
				result.TokenDebited = true
				result.TokenKey = token.Key
			}
			switch source {
			case "wallet":
				update := tx.Model(&User{}).
					Where("id = ? AND quota >= ?", attempt.UserID, heldQuota).
					Update("quota", gorm.Expr("quota - ?", heldQuota))
				if update.Error != nil {
					return update.Error
				}
				if update.RowsAffected != 1 {
					return fmt.Errorf("%w: wallet", ErrTaskAttemptInsufficientQuota)
				}
			case "subscription":
				subResult, plan, err := preConsumeTaskAttemptSubscriptionTx(
					tx, attempt.AttemptID, attempt.UserID, params.ModelName, int64(heldQuota),
				)
				if err != nil {
					return err
				}
				result.SubscriptionID = subResult.UserSubscriptionId
				result.SubscriptionPreConsumed = subResult.PreConsumed
				result.SubscriptionAmountTotal = subResult.AmountTotal
				result.SubscriptionAmountUsed = subResult.AmountUsedAfter
				if plan != nil {
					result.SubscriptionPlanID = plan.Id
					result.SubscriptionPlanTitle = plan.Title
				}
			}
		}
		updated := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status = ? AND billing_hold_state = ?",
				attempt.ID, TaskCreateAttemptPrepared, TaskCreateAttemptBillingUnheld).
			Updates(map[string]any{
				"status":              TaskCreateAttemptSending,
				"billing_hold_state":  TaskCreateAttemptBillingHeld,
				"billing_source":      source,
				"subscription_id":     result.SubscriptionID,
				"held_quota":          heldQuota,
				"token_quota_tracked": result.TokenTracked,
				"token_quota_held":    result.TokenDebited,
				"updated_at":          common.GetTimestamp(),
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("task attempt state changed while reserving")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.HeldQuota > 0 {
		if source == "wallet" {
			gopool.Go(func() {
				if err := cacheDecrUserQuota(userID, int64(result.HeldQuota)); err != nil {
					common.SysLog("failed to update task attempt wallet cache: " + err.Error())
				}
			})
		}
		if result.TokenDebited && result.TokenKey != "" && common.RedisEnabled && common.RDB != nil {
			gopool.Go(func() {
				if err := cacheDecrTokenQuota(result.TokenKey, int64(result.HeldQuota)); err != nil {
					common.SysLog("failed to update task attempt token cache: " + err.Error())
				}
			})
		}
	}
	return result, nil
}

func preConsumeTaskAttemptSubscriptionTx(
	tx *gorm.DB,
	requestID string,
	userID int,
	modelName string,
	amount int64,
) (*SubscriptionPreConsumeResult, *SubscriptionPlan, error) {
	_ = modelName
	now := common.GetTimestamp()
	var subscriptions []UserSubscription
	if err := lockForUpdate(tx).
		Where("user_id = ? AND status = ? AND end_time > ?", userID, "active", now).
		Order("end_time asc, id asc").
		Find(&subscriptions).Error; err != nil {
		return nil, nil, err
	}
	for i := range subscriptions {
		subscription := subscriptions[i]
		plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
		if err != nil {
			return nil, nil, err
		}
		if err := maybeResetUserSubscriptionWithPlanTx(tx, &subscription, plan, now); err != nil {
			return nil, nil, err
		}
		usedBefore := subscription.AmountUsed
		if subscription.AmountTotal > 0 && subscription.AmountTotal-usedBefore < amount {
			continue
		}
		record := &SubscriptionPreConsumeRecord{
			RequestId:          requestID,
			UserId:             userID,
			UserSubscriptionId: subscription.Id,
			PreConsumed:        amount,
			Status:             "consumed",
		}
		if err := tx.Create(record).Error; err != nil {
			return nil, nil, err
		}
		subscription.AmountUsed += amount
		if err := tx.Save(&subscription).Error; err != nil {
			return nil, nil, err
		}
		return &SubscriptionPreConsumeResult{
			UserSubscriptionId: subscription.Id,
			PreConsumed:        amount,
			AmountTotal:        subscription.AmountTotal,
			AmountUsedBefore:   usedBefore,
			AmountUsedAfter:    subscription.AmountUsed,
		}, plan, nil
	}
	return nil, nil, fmt.Errorf("%w: subscription quota insufficient", ErrTaskAttemptSubscriptionUnavailable)
}
