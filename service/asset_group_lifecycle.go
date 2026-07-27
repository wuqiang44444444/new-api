package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const assetGroupCreationOutcomeUnknownCode = "asset_group_creation_outcome_unknown"

func automaticAssetGroupWatchdogKey(groupID int64) string {
	return fmt.Sprintf("resolve-unknown-group-create:%d", groupID)
}

func preflightAutomaticAssetGroupCreate(asset *model.Asset, binding *model.AssetBinding, adapter assetadapter.Adapter) error {
	if _, ok := adapter.(assetadapter.GroupAdapter); !ok {
		return nil
	}
	groupKind := "general_aigc"
	if asset.AssetKind == model.AssetKindRealPerson {
		groupKind = "real_person"
	}
	var group model.AssetGroupBinding
	err := model.DB.Select("status", "error_code").
		Where("scope_key = ? AND channel_id = ? AND credential_fingerprint = ? AND group_kind = ?", model.AssetScopeKey(asset.UserID, asset.AuthorizationID), binding.ChannelID, binding.CredentialFingerprint, groupKind).
		First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if group.Status == model.AssetBindingStatusPending || group.Status == model.AssetBindingStatusCreateUnknown || (group.Status == model.AssetBindingStatusFailed && group.ErrorCode == assetGroupCreationOutcomeUnknownCode) {
		return fmt.Errorf("%w: automatic asset group creation is unresolved", ErrAssetUpstreamUnavailable)
	}
	return nil
}

func prepareAutomaticAssetGroupCreate(asset *model.Asset, candidate *model.AssetGroupBinding) (*model.AssetGroupBinding, bool, error) {
	var prepared *model.AssetGroupBinding
	owned := false
	now := common.GetTimestamp()
	deadline := now + asset_setting.Current().CreateUnknownTTLSeconds
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if candidate.AuthorizationID != nil {
			authorization, err := model.LockRealPersonAuthorization(tx, *candidate.AuthorizationID)
			if err != nil {
				return err
			}
			if authorization.UserID != asset.UserID || authorization.Status != model.RealPersonAuthorizationAuthorized || authorization.RevokedAt != 0 {
				return ErrRealPersonAuthorizationNotReady
			}
		}

		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope_key"}, {Name: "channel_id"}, {Name: "credential_fingerprint"}, {Name: "group_kind"}},
			DoNothing: true,
		}).Create(candidate)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 1 {
			if err := ensureAutomaticAssetGroupWatchdogTx(tx, candidate, deadline, false); err != nil {
				return err
			}
			prepared = candidate
			owned = true
			return nil
		}

		var existing model.AssetGroupBinding
		if err := tx.Where("scope_key = ? AND channel_id = ? AND credential_fingerprint = ? AND group_kind = ?", candidate.ScopeKey, candidate.ChannelID, candidate.CredentialFingerprint, candidate.GroupKind).First(&existing).Error; err != nil {
			return err
		}
		prepared = &existing
		if existing.UpstreamResourceID == "" && (existing.Status == model.AssetBindingStatusPending || existing.Status == model.AssetBindingStatusCreateUnknown) {
			return ensureAutomaticAssetGroupWatchdogTx(tx, &existing, deadline, true)
		}
		if existing.Status == model.AssetBindingStatusProcessing {
			return ensureAutomaticAssetGroupWatchdogTx(tx, &existing, deadline, true)
		}
		if existing.UpstreamResourceID != "" || existing.Status != model.AssetBindingStatusFailed || existing.ErrorCode == assetGroupCreationOutcomeUnknownCode {
			return nil
		}

		claimed := tx.Model(&model.AssetGroupBinding{}).
			Where("id = ? AND status = ? AND upstream_resource_id = ?", existing.ID, model.AssetBindingStatusFailed, "").
			Updates(map[string]any{"status": model.AssetBindingStatusPending, "error_code": "", "error_message": "", "updated_at": now})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return nil
		}
		existing.Status = model.AssetBindingStatusPending
		existing.ErrorCode = ""
		existing.ErrorMessage = ""
		if err := ensureAutomaticAssetGroupWatchdogTx(tx, &existing, deadline, true); err != nil {
			return err
		}
		prepared = &existing
		owned = true
		return nil
	})
	return prepared, owned, err
}

func ensureAutomaticAssetGroupWatchdogTx(tx *gorm.DB, group *model.AssetGroupBinding, deadline int64, reviveTerminal bool) error {
	job := &model.AssetOperationJob{
		IdempotencyKey: automaticAssetGroupWatchdogKey(group.ID),
		Kind:           "resolve_unknown_group_create",
		GroupBindingID: &group.ID,
		NextAttemptAt:  deadline,
	}
	existing, err := model.EnsureAssetOperationJob(tx, job, false)
	if err != nil || !reviveTerminal || (existing.Status != model.AssetJobDead && existing.Status != model.AssetJobSucceeded) {
		return err
	}
	now := common.GetTimestamp()
	return tx.Model(&model.AssetOperationJob{}).
		Where("id = ? AND status IN ?", existing.ID, []string{model.AssetJobDead, model.AssetJobSucceeded}).
		Updates(map[string]any{
			"status": model.AssetJobPending, "attempt_count": 0,
			"max_attempts": asset_setting.Current().JobMaxAttempts, "next_attempt_at": deadline,
			"locked_by": "", "locked_until": int64(0), "last_error": "", "updated_at": now,
		}).Error
}

func createAutomaticAssetGroup(ctx context.Context, groupAdapter assetadapter.GroupAdapter, group *model.AssetGroupBinding, binding *model.AssetBinding) (string, error) {
	result, createErr := groupAdapter.CreateGroup(ctx, assetadapter.GroupRequest{Name: group.Name, Description: group.Description, GroupType: "AIGC"})
	if createErr != nil {
		return "", recordAssetGroupCreateError(group, createErr)
	}
	if result.ResourceID == "" {
		return "", recordAssetGroupCreateError(group, fmt.Errorf("upstream did not return an asset group id"))
	}
	usable, err := saveAutomaticAssetGroupResult(group, result)
	if err != nil {
		return "", err
	}
	if !usable {
		return "", fmt.Errorf("upstream asset group is being deleted")
	}
	if group.Status != model.AssetBindingStatusActive {
		if group.Status == model.AssetBindingStatusFailed {
			return "", fmt.Errorf("upstream asset group failed")
		}
		return "", errAssetGroupCreateOutcomeUnknown
	}
	binding.UpstreamGroupBindingID = &group.ID
	if err := model.DB.Model(binding).Update("upstream_group_binding_id", group.ID).Error; err != nil {
		return "", err
	}
	return group.UpstreamResourceID, nil
}

func saveAutomaticAssetGroupResult(group *model.AssetGroupBinding, result assetadapter.GroupResult) (bool, error) {
	status := model.AssetBindingStatusProcessing
	if result.Status == "active" {
		status = model.AssetBindingStatusActive
	} else if result.Status == "failed" {
		status = model.AssetBindingStatusFailed
	}
	now := common.GetTimestamp()
	usable := false
	var saved *model.AssetGroupBinding
	err := runAssetStateTransaction(func() error {
		usable = false
		saved = nil
		return model.DB.Transaction(func(tx *gorm.DB) error {
			authorizationActive := true
			if group.AuthorizationID != nil {
				authorization, err := model.LockRealPersonAuthorization(tx, *group.AuthorizationID)
				if err != nil {
					return err
				}
				authorizationActive = authorization.Status == model.RealPersonAuthorizationAuthorized && authorization.RevokedAt == 0
			}
			current, err := model.LockAssetGroupBinding(tx, group.ID)
			if err != nil {
				return err
			}
			if err := model.ClaimAssetGroupOwnership(tx, current, result.ResourceID); err != nil {
				return err
			}
			cleanup := !authorizationActive || current.Status == model.AssetBindingStatusDeleting || current.Status == model.AssetBindingStatusDeletionFailed || current.Status == model.AssetBindingStatusDeleted
			if cleanup {
				if err := tx.Model(current).Updates(map[string]any{
					"upstream_resource_id": result.ResourceID, "upstream_group_id": result.BusinessID,
					"upstream_request_id": result.RequestID, "status": model.AssetBindingStatusDeleting,
					"error_code": "", "error_message": "", "updated_at": now,
				}).Error; err != nil {
					return err
				}
				cleanupJob := &model.AssetOperationJob{IdempotencyKey: fmt.Sprintf("delete-group:%d", current.ID), Kind: "delete_group", GroupBindingID: &current.ID}
				if _, err := model.EnsureAssetOperationJob(tx, cleanupJob, true); err != nil {
					return err
				}
				if err := model.CompleteQueuedAssetOperationJobTx(tx, automaticAssetGroupWatchdogKey(current.ID)); err != nil {
					return err
				}
				current.UpstreamResourceID = result.ResourceID
				current.UpstreamGroupID = result.BusinessID
				current.UpstreamRequestID = result.RequestID
				current.Status = model.AssetBindingStatusDeleting
				saved = current
				return nil
			}

			if current.Status != model.AssetBindingStatusPending && current.Status != model.AssetBindingStatusCreateUnknown && current.Status != model.AssetBindingStatusFailed && current.Status != model.AssetBindingStatusProcessing {
				return fmt.Errorf("automatic asset group state changed before result persistence")
			}
			if err := tx.Model(current).Updates(map[string]any{
				"upstream_resource_id": result.ResourceID, "upstream_group_id": result.BusinessID,
				"upstream_request_id": result.RequestID, "status": status,
				"error_code": "", "error_message": "", "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if status != model.AssetBindingStatusProcessing {
				if err := model.CompleteQueuedAssetOperationJobTx(tx, automaticAssetGroupWatchdogKey(current.ID)); err != nil {
					return err
				}
			}
			current.UpstreamResourceID = result.ResourceID
			current.UpstreamGroupID = result.BusinessID
			current.UpstreamRequestID = result.RequestID
			current.Status = status
			current.ErrorCode = ""
			current.ErrorMessage = ""
			usable = true
			saved = current
			return nil
		})
	})
	if err == nil && saved != nil {
		*group = *saved
	}
	return usable, err
}

func scheduleAutomaticAssetGroupDeletionTx(tx *gorm.DB, group *model.AssetGroupBinding, now int64) error {
	updated := tx.Model(&model.AssetGroupBinding{}).
		Where("id = ? AND status <> ?", group.ID, model.AssetBindingStatusDeleted).
		Updates(map[string]any{"status": model.AssetBindingStatusDeleting, "updated_at": now})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected == 0 {
		return nil
	}
	group.Status = model.AssetBindingStatusDeleting
	job := &model.AssetOperationJob{IdempotencyKey: fmt.Sprintf("delete-group:%d", group.ID), Kind: "delete_group", GroupBindingID: &group.ID, Status: model.AssetJobPending}
	_, err := model.EnsureAssetOperationJob(tx, job, true)
	return err
}

func resolveUnknownAssetGroupCreate(job *model.AssetOperationJob) error {
	if job.GroupBindingID == nil {
		return fmt.Errorf("group binding id is required")
	}
	var group model.AssetGroupBinding
	if err := model.DB.Select("id", "authorization_id").First(&group, "id = ?", *job.GroupBindingID).Error; err != nil {
		return err
	}
	expired := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		expired, err = expireUnknownAssetGroupCreateTx(tx, &group)
		if err != nil {
			return err
		}
		return model.FinishAssetOperationJobTx(tx, job.ID, job.LockedBy)
	})
	if err == nil && expired {
		common.SysError(fmt.Sprintf("automatic asset group create outcome remains unknown: group_id=%d orphan_suspected=true", group.ID))
	}
	return err
}

func expireUnknownAssetGroupCreateTx(tx *gorm.DB, group *model.AssetGroupBinding) (bool, error) {
	if group.AuthorizationID != nil {
		if _, err := model.LockRealPersonAuthorization(tx, *group.AuthorizationID); err != nil {
			return false, err
		}
	}
	current, err := model.LockAssetGroupBinding(tx, group.ID)
	if err != nil {
		return false, err
	}
	if current.Status == model.AssetBindingStatusProcessing {
		return true, tx.Model(current).Updates(map[string]any{
			"status": model.AssetBindingStatusFailed, "error_code": "asset_group_processing_timeout",
			"error_message": "upstream asset group processing did not finish in time", "updated_at": common.GetTimestamp(),
		}).Error
	}
	if current.UpstreamResourceID != "" {
		return false, nil
	}
	if current.Status == model.AssetBindingStatusPending || current.Status == model.AssetBindingStatusCreateUnknown {
		return true, tx.Model(current).Updates(map[string]any{
			"status": model.AssetBindingStatusFailed, "error_code": assetGroupCreationOutcomeUnknownCode,
			"error_message": "upstream asset group creation outcome could not be confirmed", "updated_at": common.GetTimestamp(),
		}).Error
	}
	return (current.Status == model.AssetBindingStatusDeleting || current.Status == model.AssetBindingStatusDeletionFailed) && current.ErrorCode == assetGroupCreationOutcomeUnknownCode, nil
}

func expireDeadUnknownAssetGroupCreate(job *model.AssetOperationJob) error {
	if job.GroupBindingID == nil {
		return fmt.Errorf("group binding id is required")
	}
	var group model.AssetGroupBinding
	if err := model.DB.Select("id", "authorization_id").First(&group, "id = ?", *job.GroupBindingID).Error; err != nil {
		return err
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		_, err := expireUnknownAssetGroupCreateTx(tx, &group)
		return err
	})
}
