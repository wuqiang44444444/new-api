package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// RestoreBillingReportGeneration rolls back one publication while preserving
// every currently published customer funding event. A more recent head is
// never overwritten by an operator acting on an older manifest.
func RestoreBillingReportGeneration(ctx context.Context, date, expected, restore string) error {
	day, err := time.ParseInLocation(time.DateOnly, date, UsageAnalyticsZone())
	if err != nil || day.Unix() <= 0 || expected == "" || restore == "" || expected == restore {
		return common.NewMessage("Invalid billing report rollback")
	}
	if err := verifyBillingReportFunding(ctx, expected, restore); err != nil {
		return err
	}
	if err := verifyBillingReportFunding(ctx, restore, expected); err != nil {
		return err
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var head billingReportHead
		if err := lockForUpdate(tx).Where("start = ? AND generation = ?", day.Unix(), expected).Take(&head).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewMessage("Billing report changed since review")
			}
			return err
		}
		var current, previous billingReportGeneration
		if err := lockForUpdate(tx).Where("id = ? AND published = ?", expected, true).Take(&current).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ? AND start = ? AND published = ? AND retired_at > 0", restore, day.Unix(), true).Take(&previous).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewMessage("Previous billing report generation is no longer available")
			}
			return err
		}
		if previous.Rows != current.Rows || (previous.Incomplete && !current.Incomplete) {
			return common.NewMessage("Previous billing report does not cover the published source")
		}
		if err := tx.Model(&current).Update("retired_at", common.GetTimestamp()).Error; err != nil {
			return err
		}
		if err := tx.Model(&previous).Update("retired_at", 0).Error; err != nil {
			return err
		}
		if err := tx.Where("start = ? AND kind = ?", day.Unix(), DailyReportBilling).Delete(&dailyReportStage{}).Error; err != nil {
			return err
		}
		return tx.Model(&head).Updates(map[string]any{"generation": previous.ID, "as_of": previous.AsOf}).Error
	})
}
