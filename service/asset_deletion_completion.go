package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func completeAssetBindingDeletion(job *model.AssetOperationJob, binding *model.AssetBinding, asset *model.Asset) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if asset.AuthorizationID != nil {
			if _, err := model.LockRealPersonAuthorization(tx, *asset.AuthorizationID); err != nil {
				return err
			}
		}
		currentAsset, err := model.LockAsset(tx, asset.ID)
		if err != nil {
			return err
		}
		updated := tx.Model(&model.AssetBinding{}).
			Where("id = ? AND status IN ? AND upstream_resource_id = ?", binding.ID, []string{model.AssetBindingStatusDeleting, model.AssetBindingStatusDeletionFailed}, binding.UpstreamResourceID).
			Updates(map[string]any{"status": model.AssetBindingStatusDeleted, "error_code": "", "error_message": "", "updated_at": common.GetTimestamp()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := finalizeAssetDeletionIfReadyTx(tx, currentAsset); err != nil {
			return err
		}
		return model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy)
	})
}

func completeAssetGroupDeletion(job *model.AssetOperationJob, group *model.AssetGroupBinding) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if group.AuthorizationID != nil {
			if _, err := model.LockRealPersonAuthorization(tx, *group.AuthorizationID); err != nil {
				return err
			}
		}
		updated := tx.Model(&model.AssetGroupBinding{}).
			Where("id = ? AND status IN ? AND upstream_resource_id = ?", group.ID, []string{model.AssetBindingStatusDeleting, model.AssetBindingStatusDeletionFailed}, group.UpstreamResourceID).
			Updates(map[string]any{"status": model.AssetBindingStatusDeleted, "error_code": "", "error_message": "", "updated_at": common.GetTimestamp()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := model.CompleteQueuedAssetOperationJobTx(tx, automaticAssetGroupWatchdogKey(group.ID)); err != nil {
			return err
		}
		if err := finalizeRealPersonAuthorizationIfCleanTx(tx, group.AuthorizationID); err != nil {
			return err
		}
		return model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy)
	})
}

func markAssetBindingDeletionFailed(binding *model.AssetBinding, asset *model.Asset, code, message string) error {
	if asset.AuthorizationID == nil {
		if err := model.DB.Select("authorization_id").First(asset, "id = ?", asset.ID).Error; err != nil {
			return err
		}
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		currentAsset, currentBinding, _, err := lockRemoteCreateState(tx, asset, binding)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if result := tx.Model(currentBinding).
			Where("status IN ?", []string{model.AssetBindingStatusDeleting, model.AssetBindingStatusDeletionFailed}).
			Updates(map[string]any{"status": model.AssetBindingStatusDeletionFailed, "error_code": code, "error_message": message, "updated_at": now}); result.Error != nil {
			return result.Error
		}
		return tx.Model(currentAsset).
			Where("status IN ?", []string{model.AssetStatusDeleting, model.AssetStatusDeletionFailed}).
			Updates(map[string]any{"status": model.AssetStatusDeletionFailed, "error_code": code, "error_message": message, "updated_at": now}).Error
	})
}

func finalizeAssetDeletionIfReadyTx(tx *gorm.DB, asset *model.Asset) error {
	var count int64
	if err := tx.Model(&model.AssetBinding{}).Where("asset_id = ? AND status <> ?", asset.ID, model.AssetBindingStatusDeleted).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	now := common.GetTimestamp()
	if err := tx.Model(asset).Updates(map[string]any{"status": model.AssetStatusDeleted, "deleted_at": now, "updated_at": now, "error_code": "", "error_message": ""}).Error; err != nil {
		return err
	}
	return finalizeRealPersonAuthorizationIfCleanTx(tx, asset.AuthorizationID)
}

func finalizeRealPersonAuthorizationIfCleanTx(tx *gorm.DB, authorizationID *int64) error {
	if authorizationID == nil {
		return nil
	}
	var remainingAssets int64
	if err := tx.Model(&model.Asset{}).Where("authorization_id = ? AND deleted_at = ?", *authorizationID, 0).Count(&remainingAssets).Error; err != nil {
		return err
	}
	var remainingGroups int64
	if err := tx.Model(&model.AssetGroupBinding{}).Where("authorization_id = ? AND status <> ?", *authorizationID, model.AssetBindingStatusDeleted).Count(&remainingGroups).Error; err != nil {
		return err
	}
	if remainingAssets == 0 && remainingGroups == 0 {
		return tx.Model(&model.RealPersonAuthorization{}).Where("id = ? AND status = ?", *authorizationID, model.RealPersonAuthorizationRevoked).Updates(map[string]any{"status": model.RealPersonAuthorizationDeleted, "error_code": "", "updated_at": common.GetTimestamp()}).Error
	}
	return nil
}
