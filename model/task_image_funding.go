package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Native image acceptance uses the native billing preference, inside the same
// transaction as Task, token quota, capacity and idempotency ownership.
// Keep preference semantics aligned with service.holdTaskAttemptForBilling;
// that attempt-owning transaction cannot be nested in image Task acceptance.
func holdNativeImageFundingTx(tx *gorm.DB, task *Task, preference string) error {
	if task.PrivateData.ImageTask.FreeModel {
		return nil
	}
	pref := common.NormalizeBillingPreference(preference)
	if pref == "wallet_only" {
		return holdImageWalletTx(tx, task)
	}
	if pref == "wallet_first" {
		err := holdImageWalletTx(tx, task)
		if !errors.Is(err, ErrTaskAttemptInsufficientQuota) {
			return err
		}
		return holdImageSubscriptionTx(tx, task)
	}
	if pref == "subscription_only" {
		return holdImageSubscriptionTx(tx, task)
	}
	// Read through this transaction (including SQLite); no nested/global DB query.
	var subscriptions []UserSubscription
	if err := lockForUpdate(tx).Where("user_id = ? AND status = ? AND end_time > ?", task.UserId, "active", common.GetTimestamp()).Find(&subscriptions).Error; err != nil {
		return err
	}
	if len(subscriptions) == 0 {
		return holdImageWalletTx(tx, task)
	}
	err := holdImageSubscriptionTx(tx, task)
	if !errors.Is(err, ErrTaskAttemptSubscriptionUnavailable) {
		return err
	}
	for _, sub := range subscriptions {
		if !sub.AllowWalletOverflow {
			return err
		}
	}
	return holdImageWalletTx(tx, task)
}

func holdImageWalletTx(tx *gorm.DB, task *Task) error {
	if task.Quota == 0 {
		// MySQL reports zero affected rows for quota = quota - 0. Validate the
		// native positive-wallet requirement without interpreting that as failure.
		var user User
		err := lockForUpdate(tx).Select("id").Where("id = ? AND quota > 0", task.UserId).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTaskAttemptInsufficientQuota
		}
		if err != nil {
			return err
		}
		task.PrivateData.BillingSource = "wallet"
		return nil
	}
	wallet := tx.Model(&User{}).Where("id = ? AND quota > 0 AND quota >= ?", task.UserId, task.Quota).
		Update("quota", gorm.Expr("quota - ?", task.Quota))
	if wallet.Error != nil {
		return wallet.Error
	}
	if wallet.RowsAffected != 1 {
		return ErrTaskAttemptInsufficientQuota
	}
	task.PrivateData.BillingSource = "wallet"
	return nil
}

func holdImageSubscriptionTx(tx *gorm.DB, task *Task) error {
	amount := task.Quota
	if amount == 0 {
		amount = 1
	}
	result, _, err := preConsumeTaskAttemptSubscriptionTx(tx, task.TaskID, task.UserId, task.Properties.OriginModelName, int64(amount))
	if err != nil {
		return err
	}
	task.Quota = amount
	task.PrivateData.BillingSource = "subscription"
	task.PrivateData.SubscriptionId = result.UserSubscriptionId
	// From this same commit onward, only task settlement may release the hold.
	return tx.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ? AND status = ?", task.TaskID, "consumed").
		Updates(map[string]any{"status": "transferred", "updated_at": common.GetTimestamp()}).Error
}
