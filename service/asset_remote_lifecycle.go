package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func syncRemoteAssetStatus(assetID int64, bindingStatus, errorCode, errorMessage string) error {
	status := model.AssetStatusProcessing
	switch bindingStatus {
	case model.AssetBindingStatusActive:
		status = model.AssetStatusReady
	case model.AssetBindingStatusCreating:
		status = model.AssetStatusCreating
	case model.AssetBindingStatusCreateUnknown:
		status = model.AssetStatusCreateUnknown
	case model.AssetBindingStatusFailed, model.AssetBindingStatusStaleCredential:
		status = model.AssetStatusFailed
	}
	updates := map[string]any{"status": status, "error_code": errorCode, "error_message": errorMessage, "updated_at": common.GetTimestamp()}
	if status == model.AssetStatusReady || status == model.AssetStatusProcessing {
		updates["error_code"] = ""
		updates["error_message"] = ""
	}
	updated := model.DB.Model(&model.Asset{}).
		Where("id = ? AND status NOT IN ?", assetID, []string{model.AssetStatusDeleting, model.AssetStatusDeletionFailed, model.AssetStatusDeleted}).
		Updates(updates)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected == 0 {
		return nil
	}
	return updateAssetCreateIdempotency(model.DB, assetID, status)
}

func resolveUnknownRemoteCreate(job *model.AssetOperationJob) error {
	if job.AssetID == nil || job.BindingID == nil {
		return fmt.Errorf("asset and binding ids are required")
	}
	now := common.GetTimestamp()
	expiredUnknownOutcome := false
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		expiredUnknownOutcome, err = expireUnknownRemoteCreateTx(tx, *job.AssetID, *job.BindingID, now)
		if err != nil {
			return err
		}
		return model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy)
	}); err != nil {
		return err
	}
	if expiredUnknownOutcome {
		common.SysError(fmt.Sprintf("remote asset create outcome remains unknown: asset_id=%d orphan_suspected=true", *job.AssetID))
	}
	return nil
}

func expireUnknownRemoteCreateTx(tx *gorm.DB, assetID, bindingID, now int64) (bool, error) {
	assetUpdate := tx.Model(&model.Asset{}).Where("id = ? AND status IN ?", assetID, []string{model.AssetStatusCreating, model.AssetStatusCreateUnknown}).Updates(map[string]any{
		"status": model.AssetStatusFailed, "error_code": "asset_creation_outcome_unknown",
		"error_message": "upstream creation outcome could not be confirmed", "updated_at": now,
	})
	if assetUpdate.Error != nil {
		return false, assetUpdate.Error
	}
	if assetUpdate.RowsAffected == 0 {
		var unresolvedBinding int64
		if err := tx.Model(&model.AssetBinding{}).Where("id = ? AND status IN ?", bindingID, []string{model.AssetBindingStatusCreating, model.AssetBindingStatusCreateUnknown}).Count(&unresolvedBinding).Error; err != nil {
			return false, err
		}
		if unresolvedBinding != 0 {
			return false, fmt.Errorf("remote asset create states changed inconsistently")
		}
		return false, nil
	}
	bindingUpdate := tx.Model(&model.AssetBinding{}).Where("id = ? AND status IN ?", bindingID, []string{model.AssetBindingStatusCreating, model.AssetBindingStatusCreateUnknown}).Updates(map[string]any{
		"status": model.AssetBindingStatusFailed, "error_code": "asset_creation_outcome_unknown",
		"error_message": "upstream creation outcome could not be confirmed", "updated_at": now,
	})
	if bindingUpdate.Error != nil {
		return false, bindingUpdate.Error
	}
	if bindingUpdate.RowsAffected != 1 {
		return false, fmt.Errorf("remote asset create state changed while resolving unknown outcome")
	}
	if err := updateAssetCreateIdempotency(tx, assetID, model.AssetStatusFailed); err != nil {
		return false, err
	}
	return true, nil
}

func expireDeadUnknownRemoteCreate(job *model.AssetOperationJob) error {
	if job.AssetID == nil || job.BindingID == nil {
		return fmt.Errorf("asset and binding ids are required")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		_, err := expireUnknownRemoteCreateTx(tx, *job.AssetID, *job.BindingID, common.GetTimestamp())
		return err
	})
}
