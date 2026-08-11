package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ProviderCostExposureSourceTask = "task"

type ProviderCostExposure struct {
	ID                    int64   `json:"-" gorm:"primaryKey"`
	SourceKind            string  `json:"source_kind" gorm:"type:varchar(32);uniqueIndex:idx_provider_exposure_source"`
	SourceID              string  `json:"source_id" gorm:"type:varchar(191);uniqueIndex:idx_provider_exposure_source"`
	Reason                string  `json:"reason" gorm:"type:varchar(64);uniqueIndex:idx_provider_exposure_source;index"`
	UserID                int     `json:"user_id" gorm:"index"`
	ChannelID             int     `json:"channel_id" gorm:"index"`
	PublicModel           string  `json:"public_model" gorm:"type:varchar(191);index"`
	UpstreamProfile       string  `json:"upstream_profile" gorm:"type:varchar(64);index"`
	CustomerQuotaReleased int     `json:"customer_quota_released"`
	ProviderAmount        *string `json:"provider_amount,omitempty" gorm:"type:varchar(64)"`
	ProviderCurrency      string  `json:"provider_currency,omitempty" gorm:"type:varchar(16)"`
	CreatedAt             int64   `json:"created_at" gorm:"bigint;index"`
}

func insertProviderCostExposureTx(tx *gorm.DB, exposure *ProviderCostExposure) error {
	if exposure == nil {
		return nil
	}
	if tx == nil || strings.TrimSpace(exposure.SourceKind) == "" ||
		strings.TrimSpace(exposure.SourceID) == "" || strings.TrimSpace(exposure.Reason) == "" ||
		exposure.CustomerQuotaReleased < 0 {
		return errors.New("provider cost exposure is incomplete")
	}
	if exposure.CreatedAt == 0 {
		exposure.CreatedAt = common.GetTimestamp()
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(exposure).Error
}
