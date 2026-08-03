package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAssetOwnershipConflict = errors.New("upstream asset is already claimed by another binding")

func ClaimAssetOwnership(tx *gorm.DB, binding *AssetBinding, upstreamResourceID string) error {
	if tx == nil || binding == nil || binding.ID == 0 || strings.TrimSpace(upstreamResourceID) == "" {
		return nil
	}
	claim := AssetOwnershipClaim{
		ProviderAccountFingerprint: binding.CredentialFingerprint,
		UpstreamProfile:            binding.UpstreamProfile,
		ProviderProject:            binding.ProviderProject,
		Region:                     binding.Region,
		UpstreamResourceID:         strings.TrimSpace(upstreamResourceID),
		AssetBindingID:             binding.ID,
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
	var existing AssetOwnershipClaim
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
	if existing.AssetBindingID == binding.ID && existing.UserID == binding.UserID {
		return nil
	}
	return ErrAssetOwnershipConflict
}
