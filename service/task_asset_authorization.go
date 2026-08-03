package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var ErrTaskContentAuthorizationRevoked = errors.New("task content authorization has been revoked")

// AuthorizeTaskContent fails closed for tasks that used real-person assets.
// It intentionally does not cache, so revocation is visible before any URL
// parsing, Data URL decoding, or provider/CDN request.
func AuthorizeTaskContent(task *model.Task) error {
	if task == nil {
		return errors.New("task is required")
	}
	publicIDs := task.PrivateData.AssetPublicIDs
	if len(publicIDs) == 0 {
		return nil
	}
	expectedAssets := make(map[string]struct{}, len(publicIDs))
	for _, publicID := range publicIDs {
		if publicID == "" {
			return ErrTaskContentAuthorizationRevoked
		}
		expectedAssets[publicID] = struct{}{}
	}
	var assets []model.Asset
	if err := model.DB.Unscoped().
		Where("user_id = ? AND app_id = ? AND public_id IN ?", task.UserId, task.PrivateData.AppID, publicIDs).
		Find(&assets).Error; err != nil {
		return err
	}
	if len(assets) != len(expectedAssets) {
		return ErrTaskContentAuthorizationRevoked
	}
	realPersonAssets := make(map[int64]int64)
	for i := range assets {
		if assets[i].AssetKind == model.AssetKindRealPerson {
			if assets[i].AuthorizationID == nil || assets[i].EndUserSubjectHash != task.PrivateData.EndUserSubjectHash {
				return ErrTaskContentAuthorizationRevoked
			}
			realPersonAssets[assets[i].ID] = *assets[i].AuthorizationID
		}
	}
	if len(realPersonAssets) == 0 {
		return nil
	}
	if task.PrivateData.AppID <= 0 || task.PrivateData.EndUserSubjectHash == "" {
		return ErrTaskContentAuthorizationRevoked
	}
	var relations []model.TaskAssetAuthorization
	if err := model.DB.
		Where("user_id = ? AND app_id = ? AND end_user_subject_hash = ? AND task_id = ? AND state = ?",
			task.UserId, task.PrivateData.AppID, task.PrivateData.EndUserSubjectHash, task.TaskID, model.TaskAssetAuthorizationTaskBound).
		Find(&relations).Error; err != nil {
		return err
	}
	if len(relations) != len(realPersonAssets) {
		return ErrTaskContentAuthorizationRevoked
	}
	for i := range relations {
		authorizationID, exists := realPersonAssets[relations[i].AssetID]
		if !exists || authorizationID != relations[i].AuthorizationID ||
			relations[i].AssetKind != model.AssetKindRealPerson {
			return ErrTaskContentAuthorizationRevoked
		}
	}
	active, err := model.TaskContentAuthorizationActive(task.UserId, task.PrivateData.AppID, task.PrivateData.EndUserSubjectHash, task.TaskID)
	if err != nil {
		return err
	}
	if !active {
		return ErrTaskContentAuthorizationRevoked
	}
	return nil
}

func scheduleRevokedTaskAuthorizationsTx(tx *gorm.DB, authorizationID int64, now int64) error {
	if tx == nil || authorizationID == 0 {
		return nil
	}
	var attemptIDs []string
	if err := tx.Model(&model.TaskAssetAuthorization{}).
		Where("authorization_id = ? AND state = ?",
			authorizationID, model.TaskAssetAuthorizationReserved).
		Pluck("attempt_id", &attemptIDs).Error; err != nil {
		return err
	}
	if len(attemptIDs) > 0 {
		if err := tx.Model(&model.TaskCreateAttempt{}).
			Where("attempt_id IN ? AND status IN ?", attemptIDs, []model.TaskCreateAttemptStatus{
				model.TaskCreateAttemptSending,
				model.TaskCreateAttemptUnknown,
				model.TaskCreateAttemptUpstreamSucceeded,
			}).
			Updates(map[string]any{"next_attempt_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
	}

	var taskIDs []string
	if err := tx.Model(&model.TaskAssetAuthorization{}).
		Where("authorization_id = ? AND state = ?",
			authorizationID, model.TaskAssetAuthorizationTaskBound).
		Pluck("task_id", &taskIDs).Error; err != nil {
		return err
	}
	if len(taskIDs) == 0 {
		return nil
	}
	return tx.Model(&model.Task{}).
		Where("task_id IN ? AND status IN ?", taskIDs, []model.TaskStatus{
			model.TaskStatusNotStart,
			model.TaskStatusSubmitted,
			model.TaskStatusQueued,
			model.TaskStatusInProgress,
			model.TaskStatusUnknown,
			model.TaskStatusReconciliationRequired,
		}).
		Where("(cancellation_state = ? OR cancellation_state IS NULL)", "").
		Updates(map[string]any{
			"cancellation_state":        model.TaskCancellationStateRequested,
			"cancellation_requested_at": now,
			"updated_at":                now,
		}).Error
}

func ReconcileRevokedTaskAssetAuthorizations(ctx context.Context, limit int) int {
	relations := model.GetRevokedTaskAssetAuthorizationWork(limit)
	processed := 0
	for _, relation := range relations {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		now := common.GetTimestamp()
		switch relation.State {
		case model.TaskAssetAuthorizationReserved:
			var attempt model.TaskCreateAttempt
			err := model.DB.Where("attempt_id = ?", relation.AttemptID).First(&attempt).Error
			if errors.Is(err, gorm.ErrRecordNotFound) ||
				err == nil && (attempt.Status == model.TaskCreateAttemptRejected ||
					attempt.Status == model.TaskCreateAttemptReleasedWithExposure) {
				if closeErr := closeTaskAssetAuthorizationRelation(relation.ID, now); closeErr != nil {
					logger.LogWarn(ctx, "close revoked task reservation failed: "+closeErr.Error())
					continue
				}
				processed++
				continue
			}
			if err != nil {
				logger.LogWarn(ctx, "load revoked task attempt failed: "+err.Error())
				continue
			}
			if attempt.Status == model.TaskCreateAttemptComplete {
				if err := reconcileRevokedAuthorizedTask(relation, now); err != nil {
					logger.LogWarn(ctx, "schedule newly bound revoked task cancellation failed: "+err.Error())
					continue
				}
				processed++
				continue
			}
			if err := model.ScheduleTaskCreateAttemptReconcile(attempt.ID, attempt.Status, now); err != nil {
				logger.LogWarn(ctx, "schedule revoked task attempt reconciliation failed: "+err.Error())
				continue
			}
			processed++
		case model.TaskAssetAuthorizationTaskBound:
			if err := reconcileRevokedAuthorizedTask(relation, now); err != nil {
				logger.LogWarn(ctx, "schedule revoked authorized task cancellation failed: "+err.Error())
				continue
			}
			processed++
		}
	}
	return processed
}

func reconcileRevokedAuthorizedTask(relation *model.TaskAssetAuthorization, now int64) error {
	var task model.Task
	err := model.DB.Where("user_id = ? AND task_id = ?", relation.UserID, relation.TaskID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && task.Status.IsTerminal() {
		return closeTaskAssetAuthorizationRelation(relation.ID, now)
	}
	if err != nil {
		return err
	}
	if relation.State == model.TaskAssetAuthorizationReserved {
		if err := model.DB.Model(&model.TaskAssetAuthorization{}).
			Where("id = ? AND state = ?", relation.ID, model.TaskAssetAuthorizationReserved).
			Updates(map[string]any{
				"state":      model.TaskAssetAuthorizationTaskBound,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
	}
	return model.DB.Model(&model.Task{}).
		Where("id = ? AND status IN ?", task.ID, []model.TaskStatus{
			model.TaskStatusNotStart,
			model.TaskStatusSubmitted,
			model.TaskStatusQueued,
			model.TaskStatusInProgress,
			model.TaskStatusUnknown,
			model.TaskStatusReconciliationRequired,
		}).
		Where("(cancellation_state = ? OR cancellation_state IS NULL)", "").
		Updates(map[string]any{
			"cancellation_state":        model.TaskCancellationStateRequested,
			"cancellation_requested_at": now,
			"updated_at":                now,
		}).Error
}

func closeTaskAssetAuthorizationRelation(id int64, now int64) error {
	return model.DB.Model(&model.TaskAssetAuthorization{}).
		Where("id = ? AND state IN ?", id, []string{
			model.TaskAssetAuthorizationReserved,
			model.TaskAssetAuthorizationTaskBound,
		}).
		Updates(map[string]any{"state": model.TaskAssetAuthorizationClosed, "updated_at": now}).Error
}
