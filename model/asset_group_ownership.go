package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAssetGroupOwnershipConflict = errors.New("upstream asset group is already claimed by another binding")

type AssetGroupOwnershipClaim struct {
	ID                         int64  `json:"-" gorm:"primaryKey"`
	ProviderAccountFingerprint string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_group_ownership_scope"`
	UpstreamProfile            string `json:"-" gorm:"type:varchar(32);uniqueIndex:idx_asset_group_ownership_scope"`
	ProviderProject            string `json:"-" gorm:"type:varchar(128);uniqueIndex:idx_asset_group_ownership_scope"`
	Region                     string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_asset_group_ownership_scope"`
	UpstreamResourceID         string `json:"-" gorm:"type:varchar(191);uniqueIndex:idx_asset_group_ownership_scope"`
	AssetGroupBindingID        int64  `json:"-" gorm:"uniqueIndex"`
	UserID                     int    `json:"-" gorm:"index"`
	CreatedAt                  int64  `json:"-" gorm:"bigint"`
}

func ClaimAssetGroupOwnership(tx *gorm.DB, binding *AssetGroupBinding, upstreamResourceID string) error {
	if tx == nil || binding == nil || binding.ID == 0 || strings.TrimSpace(upstreamResourceID) == "" {
		return nil
	}
	claim := AssetGroupOwnershipClaim{
		ProviderAccountFingerprint: binding.CredentialFingerprint,
		UpstreamProfile:            binding.UpstreamProfile,
		ProviderProject:            binding.ProviderProject,
		Region:                     binding.Region,
		UpstreamResourceID:         strings.TrimSpace(upstreamResourceID),
		AssetGroupBindingID:        binding.ID,
		UserID:                     binding.UserID,
		CreatedAt:                  common.GetTimestamp(),
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claim)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing AssetGroupOwnershipClaim
	err := tx.Where(
		"provider_account_fingerprint = ? AND upstream_profile = ? AND provider_project = ? AND region = ? AND upstream_resource_id = ?",
		claim.ProviderAccountFingerprint,
		claim.UpstreamProfile,
		claim.ProviderProject,
		claim.Region,
		claim.UpstreamResourceID,
	).First(&existing).Error
	if err != nil {
		return err
	}
	if existing.AssetGroupBindingID == binding.ID && existing.UserID == binding.UserID {
		return nil
	}
	return ErrAssetGroupOwnershipConflict
}
