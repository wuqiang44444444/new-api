package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"gorm.io/gorm"
)

var errAssetGroupCreateOutcomeUnknown = errors.New("asset group creation outcome is unknown")

func recordAssetGroupCreateError(group *model.AssetGroupBinding, createErr error) error {
	status := model.AssetBindingStatusFailed
	code := "upstream_group_create_failed"
	if !assetadapter.IsDefinitiveUpstreamRejection(createErr) {
		status = model.AssetBindingStatusCreateUnknown
		code = assetGroupCreationOutcomeUnknownCode
	}
	var saved *model.AssetGroupBinding
	err := runAssetStateTransaction(func() error {
		saved = nil
		return model.DB.Transaction(func(tx *gorm.DB) error {
			if group.AuthorizationID != nil {
				if _, err := model.LockRealPersonAuthorization(tx, *group.AuthorizationID); err != nil {
					return err
				}
			}
			current, err := model.LockAssetGroupBinding(tx, group.ID)
			if err != nil {
				return err
			}
			if current.Status == model.AssetBindingStatusDeleting || current.Status == model.AssetBindingStatusDeletionFailed || current.Status == model.AssetBindingStatusDeleted {
				if status == model.AssetBindingStatusCreateUnknown {
					if err := tx.Model(current).Updates(map[string]any{"error_code": code, "error_message": "upstream asset group operation failed", "updated_at": common.GetTimestamp()}).Error; err != nil {
						return err
					}
					current.ErrorCode = code
					current.ErrorMessage = "upstream asset group operation failed"
				} else if err := model.CompleteQueuedAssetOperationJobTx(tx, automaticAssetGroupWatchdogKey(current.ID)); err != nil {
					return err
				}
				saved = current
				return nil
			}
			if current.Status != model.AssetBindingStatusPending && current.Status != model.AssetBindingStatusCreateUnknown {
				saved = current
				return nil
			}
			if err := tx.Model(current).Updates(map[string]any{
				"status": status, "error_code": code, "error_message": "upstream asset group operation failed", "updated_at": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
			if status == model.AssetBindingStatusFailed {
				if err := model.CompleteQueuedAssetOperationJobTx(tx, automaticAssetGroupWatchdogKey(current.ID)); err != nil {
					return err
				}
			}
			current.Status = status
			current.ErrorCode = code
			current.ErrorMessage = "upstream asset group operation failed"
			saved = current
			return nil
		})
	})
	if err != nil {
		return err
	}
	if saved != nil {
		*group = *saved
	}
	if status == model.AssetBindingStatusCreateUnknown {
		return fmt.Errorf("%w: %v", errAssetGroupCreateOutcomeUnknown, createErr)
	}
	return createErr
}
