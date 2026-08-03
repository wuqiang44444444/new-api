package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const assetSourceScopePrefix = "asset-source:"

// AssetSource is the single protected source reference for an Asset. It has no
// public identity or lifecycle of its own.
type AssetSource struct {
	AssetID      int64  `json:"-" gorm:"primaryKey;autoIncrement:false"`
	EncryptedURL string `json:"-" gorm:"type:text;not null"`
	ExpiresAt    int64  `json:"-" gorm:"bigint"`
}

func CreateAssetSourceTx(tx *gorm.DB, asset *Asset, normalizedURL string, expiresAt int64) (*AssetSource, error) {
	if tx == nil || asset == nil || asset.ID <= 0 || strings.TrimSpace(asset.PublicID) == "" || strings.TrimSpace(normalizedURL) == "" {
		return nil, fmt.Errorf("asset source is incomplete")
	}
	encryptedURL, err := common.EncryptShortLivedSecretForScope(assetSourceScopePrefix+asset.PublicID, normalizedURL)
	if err != nil {
		return nil, err
	}
	source := &AssetSource{AssetID: asset.ID, EncryptedURL: encryptedURL, ExpiresAt: expiresAt}
	if err := tx.Create(source).Error; err != nil {
		return nil, err
	}
	return source, nil
}

func LoadAssetSources(assetIDs []int64) (map[int64]AssetSource, error) {
	result := make(map[int64]AssetSource, len(assetIDs))
	if len(assetIDs) == 0 {
		return result, nil
	}
	var sources []AssetSource
	if err := DB.Where("asset_id IN ?", assetIDs).Find(&sources).Error; err != nil {
		return nil, err
	}
	for _, source := range sources {
		result[source.AssetID] = source
	}
	return result, nil
}

func DecryptAssetSourceURL(asset *Asset, source *AssetSource) (string, error) {
	if asset == nil || source == nil || asset.ID <= 0 || source.AssetID != asset.ID || strings.TrimSpace(asset.PublicID) == "" {
		return "", fmt.Errorf("asset source scope is invalid")
	}
	// AssetSource never had an unscoped legacy format. Reject it before calling
	// the shared decryptor, whose compatibility behavior is needed elsewhere.
	if !strings.HasPrefix(source.EncryptedURL, "v2.") {
		return "", fmt.Errorf("asset source envelope is invalid")
	}
	plaintext, err := common.DecryptShortLivedSecretForScope(assetSourceScopePrefix+asset.PublicID, source.EncryptedURL)
	if err != nil {
		return "", fmt.Errorf("asset source cannot be decrypted")
	}
	return plaintext, nil
}

func DeleteAssetSourceTx(tx *gorm.DB, assetID int64) error {
	if tx == nil || assetID <= 0 {
		return nil
	}
	return tx.Delete(&AssetSource{}, "asset_id = ?", assetID).Error
}
