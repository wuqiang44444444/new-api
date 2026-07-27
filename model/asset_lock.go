package model

import "gorm.io/gorm"

func LockAsset(tx *gorm.DB, id int64) (*Asset, error) {
	var asset Asset
	if err := lockForUpdate(tx).First(&asset, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func LockAssetBinding(tx *gorm.DB, id int64) (*AssetBinding, error) {
	var binding AssetBinding
	if err := lockForUpdate(tx).First(&binding, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &binding, nil
}

func LockAssetGroupBinding(tx *gorm.DB, id int64) (*AssetGroupBinding, error) {
	var group AssetGroupBinding
	if err := lockForUpdate(tx).First(&group, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}
