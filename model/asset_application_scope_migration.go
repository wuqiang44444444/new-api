package model

import (
	"errors"

	"gorm.io/gorm"
)

// migrateAssetApplicationScope backfills only identities that can be derived
// without guessing an end user. Legacy real-person rows without a subject stay
// unusable until a new authorization is created.
func migrateAssetApplicationScope() error {
	if err := DB.Model(&RealPersonAuthorization{}).
		Where("app_id = ? AND created_by_token_id > ? AND end_user_subject_hash <> ?", 0, 0, "").
		Update("app_id", gorm.Expr("created_by_token_id")).Error; err != nil {
		return err
	}
	if err := DB.Model(&Asset{}).
		Where("app_id = ? AND created_by_token_id > ?", 0, 0).
		Update("app_id", gorm.Expr("created_by_token_id")).Error; err != nil {
		return err
	}
	if err := DB.Model(&TaskCreateAttempt{}).
		Where("app_id = ? AND token_id > ?", 0, 0).
		Update("app_id", gorm.Expr("token_id")).Error; err != nil {
		return err
	}

	var assets []Asset
	return DB.Where("asset_kind = ? AND authorization_id IS NOT NULL AND end_user_subject_hash = ?", AssetKindRealPerson, "").
		FindInBatches(&assets, 100, func(tx *gorm.DB, _ int) error {
			for i := range assets {
				var authorization RealPersonAuthorization
				if err := tx.Select("user_id", "app_id", "end_user_subject_hash").First(&authorization, "id = ?", *assets[i].AuthorizationID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					return err
				}
				if authorization.UserID != assets[i].UserID || authorization.AppID <= 0 || authorization.EndUserSubjectHash == "" {
					continue
				}
				if err := tx.Model(&Asset{}).Where("id = ? AND end_user_subject_hash = ?", assets[i].ID, "").
					Updates(map[string]any{"app_id": authorization.AppID, "end_user_subject_hash": authorization.EndUserSubjectHash}).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error
}
