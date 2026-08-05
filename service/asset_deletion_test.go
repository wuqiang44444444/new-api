package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepeatedAssetDeletionDoesNotFinishBeforeBindingsAreDeleted(t *testing.T) {
	truncate(t)
	seedUser(t, 930, 0)
	asset := model.Asset{UserID: 930, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusReady}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", UpstreamResourceID: "resource", Status: model.AssetBindingStatusActive}
	require.NoError(t, model.DB.Create(&binding).Error)

	require.NoError(t, DeleteAssetForApp(context.Background(), asset.UserID, asset.AppID, asset.PublicID))
	require.NoError(t, DeleteAssetForApp(context.Background(), asset.UserID, asset.AppID, asset.PublicID))

	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	assert.Equal(t, model.AssetStatusDeleting, asset.Status)
	assert.Zero(t, asset.DeletedAt)
	assert.Equal(t, model.AssetBindingStatusDeleting, binding.Status)
	var jobs int64
	require.NoError(t, model.DB.Model(&model.AssetOperationJob{}).Where("idempotency_key = ?", fmt.Sprintf("delete-binding:%d", binding.ID)).Count(&jobs).Error)
	assert.Equal(t, int64(1), jobs)
}

func TestAssetDeletionCompletionRollsBackJobWhenFinalizerFails(t *testing.T) {
	truncate(t)
	seedUser(t, 931, 0)
	asset := model.Asset{UserID: 931, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusDeleting}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", Status: model.AssetBindingStatusDeleting}
	require.NoError(t, model.DB.Create(&binding).Error)
	job := model.AssetOperationJob{IdempotencyKey: "delete-binding-finalizer", Kind: "delete_binding", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30}
	require.NoError(t, model.DB.Create(&job).Error)

	callbackName := "test:fail_asset_deletion_finalizer"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "assets" {
			tx.AddError(errors.New("injected asset finalizer failure"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	err := completeAssetBindingDeletion(&job, &binding, &asset)
	require.ErrorContains(t, err, "injected asset finalizer failure")
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetBindingStatusDeleting, binding.Status)
	assert.Equal(t, model.AssetJobRunning, job.Status)
}

func TestAssetDeletionCompletionDoesNotLoseLateRemoteResourceID(t *testing.T) {
	truncate(t)
	seedUser(t, 932, 0)
	asset := model.Asset{UserID: 932, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusDeleting}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", Status: model.AssetBindingStatusDeleting}
	require.NoError(t, model.DB.Create(&binding).Error)
	job := model.AssetOperationJob{IdempotencyKey: "delete-binding-late-result", Kind: "delete_binding", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30}
	require.NoError(t, model.DB.Create(&job).Error)

	staleBinding := binding
	require.NoError(t, model.DB.Model(&binding).Update("upstream_resource_id", "late-resource").Error)
	err := completeAssetBindingDeletion(&job, &staleBinding, &asset)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetBindingStatusDeleting, binding.Status)
	assert.Equal(t, "late-resource", binding.UpstreamResourceID)
	assert.Equal(t, model.AssetJobRunning, job.Status)
}

func TestRemoteCreateResultAfterDeletionStaysInCleanupState(t *testing.T) {
	truncate(t)
	seedUser(t, 936, 0)
	asset := model.Asset{UserID: 936, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusCreating}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets", Status: model.AssetBindingStatusCreating}
	require.NoError(t, model.DB.Create(&binding).Error)
	watchdog := model.AssetOperationJob{IdempotencyKey: remoteCreateWatchdogKey(binding.ID), Kind: "resolve_unknown_create", AssetID: &asset.ID, BindingID: &binding.ID}
	require.NoError(t, model.DB.Create(&watchdog).Error)

	require.NoError(t, DeleteAssetForApp(context.Background(), asset.UserID, asset.AppID, asset.PublicID))
	require.NoError(t, saveRemoteCreateResult(&asset, &binding, assetadapter.AssetResult{ResourceID: "late-resource", BusinessID: "late-business", Status: "active"}))

	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	require.NoError(t, model.DB.First(&watchdog, "id = ?", watchdog.ID).Error)
	assert.Equal(t, model.AssetStatusDeleting, asset.Status)
	assert.Equal(t, model.AssetBindingStatusDeleting, binding.Status)
	assert.Equal(t, "late-resource", binding.UpstreamResourceID)
	assert.Equal(t, model.AssetJobSucceeded, watchdog.Status)

	var deleteJob model.AssetOperationJob
	require.NoError(t, model.DB.First(&deleteJob, "idempotency_key = ?", fmt.Sprintf("delete-binding:%d", binding.ID)).Error)
	assert.Equal(t, model.AssetJobPending, deleteJob.Status)
}

func TestExhaustedBindingDeletionMarksAssetDeletionFailed(t *testing.T) {
	truncate(t)
	seedUser(t, 937, 0)
	asset := model.Asset{UserID: 937, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusDeleting}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", UpstreamResourceID: "resource", Status: model.AssetBindingStatusDeleting}
	require.NoError(t, model.DB.Create(&binding).Error)
	job := model.AssetOperationJob{IdempotencyKey: "delete-binding-exhausted", Kind: "delete_binding", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobDead}
	require.NoError(t, model.DB.Create(&job).Error)

	markAssetOperationDead(&job)

	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	assert.Equal(t, model.AssetStatusDeletionFailed, asset.Status)
	assert.Equal(t, "delete_exhausted", asset.ErrorCode)
	assert.Equal(t, model.AssetBindingStatusDeletionFailed, binding.Status)
	assert.Equal(t, "delete_exhausted", binding.ErrorCode)
}
