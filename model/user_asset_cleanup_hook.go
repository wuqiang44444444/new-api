package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// BeforeDelete makes user deletion and asset cleanup scheduling one database
// transaction. External deletion remains asynchronous and retryable.
func (user *User) BeforeDelete(tx *gorm.DB) error {
	if user.Id == 0 {
		return nil
	}
	// A database created before the asset subsystem has no related resources
	// to schedule for deletion. This also keeps rolling upgrades compatible
	// until the asset tables have been migrated.
	if !tx.Migrator().HasTable(&Asset{}) {
		return nil
	}
	// Asset creation takes this lock before authorization and channel locks.
	// Holding it through the cleanup scan guarantees a successful deletion
	// cannot miss an asset that commits concurrently.
	lockTx := tx.Unscoped()
	if err := acquireSQLiteAssetLifecycleWriteLock(lockTx, &User{}, user.Id); err != nil {
		return err
	}
	var lockedUser User
	if err := lockForUpdate(lockTx).First(&lockedUser, "id = ?", user.Id).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	if err := tx.Model(&RealPersonAuthorization{}).Where("user_id = ? AND status NOT IN ?", user.Id, []string{RealPersonAuthorizationRevoked, RealPersonAuthorizationDeleted}).Updates(map[string]any{"status": RealPersonAuthorizationRevoked, "error_code": "", "revoked_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	var assets []Asset
	if err := tx.Where("user_id = ? AND deleted_at = ?", user.Id, 0).Find(&assets).Error; err != nil {
		return err
	}
	for i := range assets {
		asset := &assets[i]
		if err := tx.Model(asset).Updates(map[string]any{"status": AssetStatusDeleting, "updated_at": now}).Error; err != nil {
			return err
		}
		var bindings []AssetBinding
		if err := tx.Where("asset_id = ? AND status <> ?", asset.ID, AssetBindingStatusDeleted).Find(&bindings).Error; err != nil {
			return err
		}
		for j := range bindings {
			bindingID := bindings[j].ID
			if err := tx.Model(&AssetBinding{}).Where("id = ?", bindingID).Update("status", AssetBindingStatusDeleting).Error; err != nil {
				return err
			}
			job := AssetOperationJob{IdempotencyKey: fmt.Sprintf("delete-binding:%d", bindingID), Kind: "delete_binding", AssetID: &asset.ID, BindingID: &bindingID, Status: AssetJobPending}
			if _, err := EnsureAssetOperationJob(tx, &job, true); err != nil {
				return err
			}
		}
		if len(bindings) == 0 {
			if err := tx.Model(asset).Updates(map[string]any{"status": AssetStatusDeleted, "deleted_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
	}
	var groups []AssetGroupBinding
	if err := tx.Where("user_id = ? AND status <> ?", user.Id, AssetBindingStatusDeleted).Find(&groups).Error; err != nil {
		return err
	}
	for i := range groups {
		groupID := groups[i].ID
		if err := tx.Model(&AssetGroupBinding{}).Where("id = ? AND status <> ?", groupID, AssetBindingStatusDeleted).Updates(map[string]any{"status": AssetBindingStatusDeleting, "updated_at": now}).Error; err != nil {
			return err
		}
		job := AssetOperationJob{IdempotencyKey: fmt.Sprintf("delete-group:%d", groupID), Kind: "delete_group", GroupBindingID: &groupID, Status: AssetJobPending}
		if _, err := EnsureAssetOperationJob(tx, &job, true); err != nil {
			return err
		}
	}
	var revokedAuthorizations []RealPersonAuthorization
	if err := tx.Select("id").Where("user_id = ? AND status = ?", user.Id, RealPersonAuthorizationRevoked).Find(&revokedAuthorizations).Error; err != nil {
		return err
	}
	for i := range revokedAuthorizations {
		authorizationID := revokedAuthorizations[i].ID
		var remainingAssets int64
		if err := tx.Model(&Asset{}).Where("authorization_id = ? AND deleted_at = ?", authorizationID, 0).Count(&remainingAssets).Error; err != nil {
			return err
		}
		var remainingGroups int64
		if err := tx.Model(&AssetGroupBinding{}).Where("authorization_id = ? AND status <> ?", authorizationID, AssetBindingStatusDeleted).Count(&remainingGroups).Error; err != nil {
			return err
		}
		if remainingAssets == 0 && remainingGroups == 0 {
			if err := tx.Model(&RealPersonAuthorization{}).Where("id = ? AND status = ?", authorizationID, RealPersonAuthorizationRevoked).Updates(map[string]any{"status": RealPersonAuthorizationDeleted, "error_code": "", "updated_at": now}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
