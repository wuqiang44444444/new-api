package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuccessfulTaskContentKeepsAuthorizationGateActiveUntilRevocation(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	authorization := &model.RealPersonAuthorization{
		UserID: 902, AppID: 1902, EndUserSubjectHash: "subject-hash-902",
		Status: model.RealPersonAuthorizationAuthorized,
	}
	require.NoError(t, model.DB.Create(authorization).Error)
	asset := &model.Asset{
		PublicID:           "asset-real-person-902",
		UserID:             902,
		AppID:              1902,
		EndUserSubjectHash: "subject-hash-902",
		AssetKind:          model.AssetKindRealPerson,
		AuthorizationID:    &authorization.ID,
	}
	require.NoError(t, model.DB.Create(asset).Error)
	task := &model.Task{
		TaskID: "task-authorized-content", UserId: 902,
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			AppID: 1902, EndUserSubjectHash: "subject-hash-902", AssetPublicIDs: []string{asset.PublicID},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	relation := &model.TaskAssetAuthorization{
		AttemptID:          "attempt-authorized-content",
		TaskID:             task.TaskID,
		UserID:             task.UserId,
		AppID:              1902,
		EndUserSubjectHash: "subject-hash-902",
		AssetID:            asset.ID,
		AuthorizationID:    authorization.ID,
		AssetKind:          model.AssetKindRealPerson,
		State:              model.TaskAssetAuthorizationTaskBound,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, model.DB.Create(relation).Error)

	require.NoError(t, AuthorizeTaskContent(task))
	require.NoError(t, model.DB.Delete(relation).Error)
	assert.True(t, errors.Is(AuthorizeTaskContent(task), ErrTaskContentAuthorizationRevoked))
	relation.ID = 0
	require.NoError(t, model.DB.Create(relation).Error)
	require.NoError(t, model.DB.Model(&model.RealPersonAuthorization{}).
		Where("id = ?", authorization.ID).
		Update("status", model.RealPersonAuthorizationRevoked).Error)
	assert.True(t, errors.Is(AuthorizeTaskContent(task), ErrTaskContentAuthorizationRevoked))
}

func TestReconcileRevokedTaskAssetAuthorizationsCoversReservationsAndTasks(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	authorization := &model.RealPersonAuthorization{
		UserID: 901,
		Status: model.RealPersonAuthorizationRevoked,
	}
	require.NoError(t, model.DB.Create(authorization).Error)

	attempt := &model.TaskCreateAttempt{
		AttemptID:        "attempt-revoked-901",
		PublicTaskID:     "task-revoked-reserved",
		UserID:           901,
		ClientProtocol:   model.TaskClientProtocolModelArkV3,
		RequestHash:      "revoked-reserved",
		Status:           model.TaskCreateAttemptUnknown,
		BillingHoldState: model.TaskCreateAttemptBillingHeld,
		HoldDeadlineAt:   now + 3600,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, model.DB.Create(attempt).Error)
	require.NoError(t, model.DB.Create(&model.TaskAssetAuthorization{
		AttemptID:       attempt.AttemptID,
		TaskID:          attempt.PublicTaskID,
		UserID:          attempt.UserID,
		AssetID:         1001,
		AuthorizationID: authorization.ID,
		AssetKind:       model.AssetKindRealPerson,
		State:           model.TaskAssetAuthorizationReserved,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	activeTask := &model.Task{
		TaskID:    "task-revoked-active",
		UserId:    901,
		Status:    model.TaskStatusInProgress,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(activeTask).Error)
	require.NoError(t, model.DB.Create(&model.TaskAssetAuthorization{
		AttemptID:       "attempt-revoked-active",
		TaskID:          activeTask.TaskID,
		UserID:          activeTask.UserId,
		AssetID:         1002,
		AuthorizationID: authorization.ID,
		AssetKind:       model.AssetKindRealPerson,
		State:           model.TaskAssetAuthorizationTaskBound,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	terminalTask := &model.Task{
		TaskID:    "task-revoked-terminal",
		UserId:    901,
		Status:    model.TaskStatusSuccess,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(terminalTask).Error)
	terminalRelation := &model.TaskAssetAuthorization{
		AttemptID:       "attempt-revoked-terminal",
		TaskID:          terminalTask.TaskID,
		UserID:          terminalTask.UserId,
		AssetID:         1003,
		AuthorizationID: authorization.ID,
		AssetKind:       model.AssetKindRealPerson,
		State:           model.TaskAssetAuthorizationTaskBound,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, model.DB.Create(terminalRelation).Error)

	newlyBoundTask := &model.Task{
		TaskID:    "task-revoked-newly-bound",
		UserId:    901,
		Status:    model.TaskStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(newlyBoundTask).Error)
	newlyBoundAttempt := &model.TaskCreateAttempt{
		AttemptID:        "attempt-revoked-newly-bound",
		PublicTaskID:     newlyBoundTask.TaskID,
		UserID:           newlyBoundTask.UserId,
		ClientProtocol:   model.TaskClientProtocolModelArkV3,
		RequestHash:      "revoked-newly-bound",
		Status:           model.TaskCreateAttemptComplete,
		BillingHoldState: model.TaskCreateAttemptBillingTransferred,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, model.DB.Create(newlyBoundAttempt).Error)
	newlyBoundRelation := &model.TaskAssetAuthorization{
		AttemptID:       newlyBoundAttempt.AttemptID,
		TaskID:          newlyBoundTask.TaskID,
		UserID:          newlyBoundTask.UserId,
		AssetID:         1004,
		AuthorizationID: authorization.ID,
		AssetKind:       model.AssetKindRealPerson,
		State:           model.TaskAssetAuthorizationReserved,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, model.DB.Create(newlyBoundRelation).Error)

	assert.Equal(t, 4, ReconcileRevokedTaskAssetAuthorizations(context.Background(), 10))

	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Positive(t, attempt.NextAttemptAt)
	require.NoError(t, model.DB.First(activeTask, activeTask.ID).Error)
	assert.Equal(t, model.TaskCancellationStateRequested, activeTask.CancellationState)
	require.NoError(t, model.DB.First(terminalRelation, terminalRelation.ID).Error)
	assert.Equal(t, model.TaskAssetAuthorizationClosed, terminalRelation.State)
	require.NoError(t, model.DB.First(newlyBoundTask, newlyBoundTask.ID).Error)
	assert.Equal(t, model.TaskCancellationStateRequested, newlyBoundTask.CancellationState)
	require.NoError(t, model.DB.First(newlyBoundRelation, newlyBoundRelation.ID).Error)
	assert.Equal(t, model.TaskAssetAuthorizationTaskBound, newlyBoundRelation.State)

	require.NoError(t, model.DB.Model(activeTask).Update("cancellation_state", model.TaskCancellationStateUnknown).Error)
	ReconcileRevokedTaskAssetAuthorizations(context.Background(), 10)
	require.NoError(t, model.DB.First(activeTask, activeTask.ID).Error)
	assert.Equal(t, model.TaskCancellationStateUnknown, activeTask.CancellationState)
}
