package model

import (
	"errors"
	"strings"

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
	return &asset, nil
}

func ListAssetsByApp(userID, appID, offset, limit int, filters ...AssetListFilter) ([]Asset, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	filter := AssetListFilter{}
	if len(filters) > 0 {
		filter = filters[0]
	}
	query := DB.Model(&Asset{}).Where("user_id = ? AND app_id = ? AND deleted_at = ?", userID, appID, 0)
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.AssetKind != "" {
		query = query.Where("asset_kind = ?", filter.AssetKind)
	}
	if filter.MediaType != "" {
		query = query.Where("media_type = ?", filter.MediaType)
	}
	if filter.Name != "" {
		query = query.Where("name LIKE ?", "%"+filter.Name+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var assets []Asset
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&assets).Error
	return assets, total, err
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
	return assets, nil
}

func GetAssetGroupByPublicIDForApp(userID, appID int, publicID string) (*AssetGroup, error) {
	var group AssetGroup
	err := DB.Where("user_id = ? AND app_id = ? AND public_id = ? AND deleted_at = ?", userID, appID, strings.TrimSpace(publicID), 0).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &group, err
}

func ListAssetGroupsByApp(userID, appID, offset, limit int) ([]AssetGroup, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := DB.Model(&AssetGroup{}).Where("user_id = ? AND app_id = ? AND deleted_at = ?", userID, appID, 0)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var groups []AssetGroup
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&groups).Error
	return groups, total, err
}
