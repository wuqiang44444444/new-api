package model

import (
	"errors"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ProviderChannelBillingDiscount is the channel-month comprehensive
// coefficient used by upstream reconciliation reference amounts. The business
// key is the billing period plus the channel: one coefficient covers every
// provider model and billing mode of the channel for that month. Provider
// model and billing mode deliberately do not participate in the discount
// identity anymore; the legacy model-level ProviderBillingDiscount table is
// historical evidence only and is never read at runtime.
type ProviderChannelBillingDiscount struct {
	Id               int64           `json:"id"`
	PeriodStart      int64           `json:"period_start" gorm:"bigint;not null;uniqueIndex:idx_provider_channel_discount_key,priority:1"`
	ChannelId        int             `json:"channel_id" gorm:"not null;uniqueIndex:idx_provider_channel_discount_key,priority:2;index"`
	PendingReason    string          `json:"pending_reason,omitempty" gorm:"type:varchar(64);not null;default:''"`
	Discount         decimal.Decimal `json:"discount" gorm:"type:decimal(12,8);not null"`
	CopiedFromPeriod int64           `json:"copied_from_period,omitempty" gorm:"bigint;not null"`
	Version          int64           `json:"version" gorm:"bigint;not null"`
	Reason           string          `json:"reason" gorm:"type:varchar(255);not null"`
	CreatedBy        int             `json:"created_by" gorm:"not null"`
	UpdatedBy        int             `json:"updated_by" gorm:"not null"`
	CreatedAt        int64           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64           `json:"updated_at" gorm:"autoUpdateTime"`
}

const (
	providerChannelDiscountEntity = "channel_discount"

	// Reasons written by automatic flows. Manual saves always carry an
	// operator-typed reason; never treat a value of 1 as confirmed no-discount
	// unless a human wrote it.
	reasonChannelDiscountDefault  = "default channel monthly coefficient"
	reasonChannelDiscountAutoCopy = "automatic copy from previous billing period"
	reasonChannelDiscountMigrated = "migrated from model-level monthly discounts"
)

// SaveProviderChannelBillingDiscount creates or updates one channel-month
// coefficient with an expected-version check. The audit row commits in the
// same transaction as the configuration change. A manual save never carries a
// copied-from period: inheritance is written only by initialization or
// migration.
func SaveProviderChannelBillingDiscount(discount *ProviderChannelBillingDiscount, expectedVersion int64, operatorId int) error {
	if discount == nil {
		return errors.New("provider channel billing discount is required")
	}
	if discount.Discount.LessThanOrEqual(decimal.Zero) || discount.Discount.GreaterThan(decimal.NewFromInt(1)) || !discount.Discount.Equal(discount.Discount.Round(8)) {
		return errors.New("discount must be between 0 and 1 with at most eight decimal places")
	}
	if discount.CopiedFromPeriod != 0 {
		return errors.New("manual channel discount cannot claim a copied period")
	}
	discount.PendingReason = ""
	return DB.Transaction(func(tx *gorm.DB) error {
		// Serialize initialization and first manual saves on an existing channel,
		// including when this month's discount row does not exist yet.
		var channel Channel
		if err := lockForUpdate(tx).Select("id").Where("id = ?", discount.ChannelId).Find(&channel).Error; err != nil {
			return err
		}
		var existing ProviderChannelBillingDiscount
		err := lockForUpdate(tx).Where("period_start = ? AND channel_id = ?", discount.PeriodStart, discount.ChannelId).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expectedVersion != 0 {
				return ErrBillingReconciliationVersionConflict
			}
			discount.Version = 1
			discount.CreatedBy = operatorId
			discount.UpdatedBy = operatorId
			if err := tx.Create(discount).Error; err != nil {
				return err
			}
			return createProviderBillingAudit(tx, providerChannelDiscountEntity, providerChannelDiscountEntityKey(discount.PeriodStart, discount.ChannelId), "create", nil, discount, discount.Reason, operatorId)
		}
		if err != nil {
			return err
		}
		if existing.Version != expectedVersion {
			return ErrBillingReconciliationVersionConflict
		}
		result := tx.Model(&ProviderChannelBillingDiscount{}).Where("id = ? AND version = ?", existing.Id, expectedVersion).Updates(map[string]interface{}{
			"discount": discount.Discount, "reason": discount.Reason,
			"version": existing.Version + 1, "updated_by": operatorId, "pending_reason": "",
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrBillingReconciliationVersionConflict
		}
		discount.Id = existing.Id
		discount.Version = existing.Version + 1
		discount.CopiedFromPeriod = existing.CopiedFromPeriod
		discount.CreatedBy = existing.CreatedBy
		discount.UpdatedBy = operatorId
		return createProviderBillingAudit(tx, providerChannelDiscountEntity, providerChannelDiscountEntityKey(discount.PeriodStart, discount.ChannelId), "update", &existing, discount, discount.Reason, operatorId)
	})
}

// GetProviderChannelBillingDiscounts is a read-only projection of the
// channel-month coefficients. Summary and export reads never materialize
// missing discounts; an absent record projects coefficient 1 at version 0.
// Persisted migration conflicts remain pending and never receive this default.
func GetProviderChannelBillingDiscounts(periodStart int64, channelIds []int) (map[int]ProviderChannelBillingDiscount, error) {
	result := make(map[int]ProviderChannelBillingDiscount)
	query := DB.Where("period_start = ?", periodStart)
	if len(channelIds) > 0 {
		query = query.Where("channel_id IN ?", channelIds)
	}
	var rows []ProviderChannelBillingDiscount
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		seen[row.ChannelId] = true
		if row.PendingReason == "" {
			result[row.ChannelId] = row
		}
	}
	for _, id := range channelIds {
		if !seen[id] {
			result[id] = ProviderChannelBillingDiscount{PeriodStart: periodStart, ChannelId: id, Discount: decimal.NewFromInt(1), Reason: reasonChannelDiscountDefault}
		}
	}
	return result, nil
}

type ProviderChannelDiscountInitOutcome struct {
	ChannelId int    `json:"channel_id"`
	Outcome   string `json:"outcome"` // created | defaulted | exists | invalid_channel
}

// InitializeProviderChannelBillingDiscounts is the explicit, idempotent
// monthly initialization: for every channel without a current-month record it
// copies the previous natural month's channel configuration into an
// independent current-month row marked with the source period. Existing
// records — manual values, corrected copies and migrated values — are never
// overwritten. Channels without a valid previous-month value initialize to 1;
// initialization never reaches further back silently.
func InitializeProviderChannelBillingDiscounts(periodStart int64, channelIds []int, operatorId int) ([]ProviderChannelDiscountInitOutcome, error) {
	uniqueIds := make([]int, 0, len(channelIds))
	seen := make(map[int]struct{}, len(channelIds))
	for _, channelId := range channelIds {
		if channelId <= 0 {
			continue
		}
		if _, ok := seen[channelId]; ok {
			continue
		}
		seen[channelId] = struct{}{}
		uniqueIds = append(uniqueIds, channelId)
	}
	sort.Ints(uniqueIds)
	outcomes := make([]ProviderChannelDiscountInitOutcome, 0, len(uniqueIds))
	if len(uniqueIds) == 0 {
		return outcomes, nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := lockForUpdate(tx).Select("id").Where("id IN ?", uniqueIds).Find(&channels).Error; err != nil {
			return err
		}
		existingChannels := make(map[int]struct{}, len(channels))
		for _, channel := range channels {
			existingChannels[channel.Id] = struct{}{}
		}

		var current []ProviderChannelBillingDiscount
		if err := tx.Where("period_start = ? AND channel_id IN ?", periodStart, uniqueIds).Find(&current).Error; err != nil {
			return err
		}
		currentChannels := make(map[int]struct{}, len(current))
		for _, row := range current {
			currentChannels[row.ChannelId] = struct{}{}
		}

		previousPeriod := previousBillingPeriodStart(periodStart)
		previous := make(map[int]ProviderChannelBillingDiscount)
		if previousPeriod > 0 {
			var rows []ProviderChannelBillingDiscount
			if err := tx.Where("period_start = ? AND channel_id IN ? AND pending_reason = ?", previousPeriod, uniqueIds, "").Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				previous[row.ChannelId] = row
			}
		}

		for _, channelId := range uniqueIds {
			outcome := ProviderChannelDiscountInitOutcome{ChannelId: channelId, Outcome: "exists"}
			if _, ok := existingChannels[channelId]; !ok {
				outcome.Outcome = "invalid_channel"
			} else if _, ok := currentChannels[channelId]; ok {
				// exists: keep manual values and previously copied months untouched.
			} else {
				source, hasSource := previous[channelId]
				discount := ProviderChannelBillingDiscount{
					PeriodStart: periodStart, ChannelId: channelId,
					Discount:         source.Discount,
					CopiedFromPeriod: previousPeriod,
					Version:          1,
					Reason:           reasonChannelDiscountAutoCopy,
					CreatedBy:        operatorId, UpdatedBy: operatorId,
				}
				if !hasSource {
					discount.Discount = decimal.NewFromInt(1)
					discount.CopiedFromPeriod = 0
					discount.Reason = reasonChannelDiscountDefault
				}
				if err := tx.Create(&discount).Error; err != nil {
					return err
				}
				if err := createProviderBillingAudit(tx, providerChannelDiscountEntity, providerChannelDiscountEntityKey(periodStart, channelId), "create", nil, &discount, discount.Reason, operatorId); err != nil {
					return err
				}
				outcome.Outcome = "created"
				if !hasSource {
					outcome.Outcome = "defaulted"
				}
			}
			outcomes = append(outcomes, outcome)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}

func providerChannelDiscountEntityKey(periodStart int64, channelId int) string {
	return fmt.Sprintf("%d:%d", periodStart, channelId)
}
