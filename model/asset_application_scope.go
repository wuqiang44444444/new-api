package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func GetAssetByPublicIDForApp(userID, appID int, publicID string) (*Asset, error) {
	var asset Asset
	err := DB.Where("user_id = ? AND app_id = ? AND public_id = ? AND deleted_at = ?", userID, appID, strings.TrimSpace(publicID), 0).First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	projected := []Asset{asset}
	if err := ProjectAssetStatuses(projected, common.GetTimestamp()); err != nil {
		return nil, err
	}
	return &projected[0], nil
}

func ListAssetsByApp(userID, appID, offset, limit int, filters ...AssetListFilter) ([]Asset, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	filter := AssetListFilter{}
	if len(filters) > 0 {
		filter = filters[0]
	}
	return listAssetsWithProjectedStatus(DB.Model(&Asset{}).Where("user_id = ? AND app_id = ? AND deleted_at = ?", userID, appID, 0), offset, limit, filter)
}

func LoadAssetsForReferenceForApp(userID, appID int, publicIDs []string) ([]Asset, error) {
	if len(publicIDs) == 0 {
		return nil, nil
	}
	var assets []Asset
	if err := DB.Where("user_id = ? AND app_id = ? AND public_id IN ? AND deleted_at = ?", userID, appID, publicIDs, 0).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(publicIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	if err := ProjectAssetStatuses(assets, common.GetTimestamp()); err != nil {
		return nil, err
	}
	return assets, nil
}
