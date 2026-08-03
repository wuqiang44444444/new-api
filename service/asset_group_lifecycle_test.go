package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAutomaticAssetGroupWatchdogClosesPendingCreateLocally(t *testing.T) {
	truncate(t)
	seedUser(t, 940, 0)
	group := model.AssetGroupBinding{
		UserID: 940, ScopeKey: model.AssetScopeKey(940, nil), ChannelID: 10,
		CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets",
		GroupKind: "general_aigc", Status: model.AssetBindingStatusPending,
	}
	require.NoError(t, model.DB.Create(&group).Error)
	job := model.AssetOperationJob{
		IdempotencyKey: automaticAssetGroupWatchdogKey(group.ID), Kind: "resolve_unknown_group_create",
		GroupBindingID: &group.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30,
	}
	require.NoError(t, model.DB.Create(&job).Error)

	require.NoError(t, resolveUnknownAssetGroupCreate(&job))

	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetBindingStatusFailed, group.Status)
	assert.Equal(t, assetGroupCreationOutcomeUnknownCode, group.ErrorCode)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestAutomaticAssetGroupProcessingResultKeepsBoundedWatchdog(t *testing.T) {
	truncate(t)
	seedUser(t, 941, 0)
	group := model.AssetGroupBinding{
		UserID: 941, ScopeKey: model.AssetScopeKey(941, nil), ChannelID: 10,
		CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets",
		GroupKind: "general_aigc", Status: model.AssetBindingStatusPending,
	}
	require.NoError(t, model.DB.Create(&group).Error)
	job := model.AssetOperationJob{
		IdempotencyKey: automaticAssetGroupWatchdogKey(group.ID), Kind: "resolve_unknown_group_create",
		GroupBindingID: &group.ID,
	}
	require.NoError(t, model.DB.Create(&job).Error)

	usable, err := saveAutomaticAssetGroupResult(&group, assetadapter.GroupResult{ResourceID: "group-resource", Status: "processing"})
	require.NoError(t, err)
	assert.True(t, usable)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetBindingStatusProcessing, group.Status)
	assert.Equal(t, model.AssetJobPending, job.Status)

	require.NoError(t, model.DB.Model(&job).Updates(map[string]any{"status": model.AssetJobRunning, "locked_by": "runner", "locked_until": int64(1 << 30)}).Error)
	job.Status = model.AssetJobRunning
	job.LockedBy = "runner"
	require.NoError(t, resolveUnknownAssetGroupCreate(&job))

	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetBindingStatusFailed, group.Status)
	assert.Equal(t, "asset_group_processing_timeout", group.ErrorCode)
	assert.Equal(t, "group-resource", group.UpstreamResourceID)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestLateAutomaticAssetGroupCreateResultQueuesCleanup(t *testing.T) {
	truncate(t)
	seedUser(t, 942, 0)
	group := model.AssetGroupBinding{
		UserID: 942, ScopeKey: model.AssetScopeKey(942, nil), ChannelID: 10,
		CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets",
		GroupKind: "general_aigc", Status: model.AssetBindingStatusDeleting,
	}
	require.NoError(t, model.DB.Create(&group).Error)
	watchdog := model.AssetOperationJob{
		IdempotencyKey: automaticAssetGroupWatchdogKey(group.ID), Kind: "resolve_unknown_group_create",
		GroupBindingID: &group.ID,
	}
	require.NoError(t, model.DB.Create(&watchdog).Error)
	deleteJob := model.AssetOperationJob{
		IdempotencyKey: fmt.Sprintf("delete-group:%d", group.ID), Kind: "delete_group", GroupBindingID: &group.ID,
		Status: model.AssetJobSucceeded,
	}
	require.NoError(t, model.DB.Create(&deleteJob).Error)

	usable, err := saveAutomaticAssetGroupResult(&group, assetadapter.GroupResult{
		ResourceID: "late-resource", BusinessID: "late-business", RequestID: "late-request", Status: "active",
	})
	require.NoError(t, err)
	assert.False(t, usable)

	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	require.NoError(t, model.DB.First(&watchdog, "id = ?", watchdog.ID).Error)
	require.NoError(t, model.DB.First(&deleteJob, "id = ?", deleteJob.ID).Error)
	assert.Equal(t, model.AssetBindingStatusDeleting, group.Status)
	assert.Equal(t, "late-resource", group.UpstreamResourceID)
	assert.Equal(t, model.AssetJobSucceeded, watchdog.Status)
	assert.Equal(t, model.AssetJobPending, deleteJob.Status)
}

func TestAutomaticAssetGroupDeletionCompletionRejectsLateResourceID(t *testing.T) {
	truncate(t)
	seedUser(t, 943, 0)
	group := model.AssetGroupBinding{
		UserID: 943, ScopeKey: model.AssetScopeKey(943, nil), ChannelID: 10,
		CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets",
		GroupKind: "general_aigc", Status: model.AssetBindingStatusDeleting,
	}
	require.NoError(t, model.DB.Create(&group).Error)
	job := model.AssetOperationJob{
		IdempotencyKey: "delete-group:late-resource", Kind: "delete_group", GroupBindingID: &group.ID,
		Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30,
	}
	require.NoError(t, model.DB.Create(&job).Error)

	staleGroup := group
	require.NoError(t, model.DB.Model(&group).Update("upstream_resource_id", "late-resource").Error)
	err := completeAssetGroupDeletion(&job, &staleGroup)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetBindingStatusDeleting, group.Status)
	assert.Equal(t, "late-resource", group.UpstreamResourceID)
	assert.Equal(t, model.AssetJobRunning, job.Status)
}

func TestScheduleAutomaticAssetGroupDeletionTransitionsBeforeEnqueue(t *testing.T) {
	truncate(t)
	seedUser(t, 944, 0)
	group := model.AssetGroupBinding{
		UserID: 944, ScopeKey: model.AssetScopeKey(944, nil), ChannelID: 10,
		CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets",
		GroupKind: "general_aigc", UpstreamResourceID: "group-resource", Status: model.AssetBindingStatusActive,
	}
	require.NoError(t, model.DB.Create(&group).Error)

	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return scheduleAutomaticAssetGroupDeletionTx(tx, &group, common.GetTimestamp())
	}))

	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	assert.Equal(t, model.AssetBindingStatusDeleting, group.Status)
	var job model.AssetOperationJob
	require.NoError(t, model.DB.First(&job, "idempotency_key = ?", fmt.Sprintf("delete-group:%d", group.ID)).Error)
	assert.Equal(t, model.AssetJobPending, job.Status)
}

func TestExhaustedAutomaticAssetGroupDeletionUsesDeletingStateFence(t *testing.T) {
	truncate(t)
	seedUser(t, 945, 0)
	group := model.AssetGroupBinding{
		UserID: 945, ScopeKey: model.AssetScopeKey(945, nil), ChannelID: 10,
		CredentialFingerprint: "credential", UpstreamProfile: "joycreator_assets",
		GroupKind: "general_aigc", UpstreamResourceID: "group-resource", Status: model.AssetBindingStatusActive,
	}
	require.NoError(t, model.DB.Create(&group).Error)
	job := model.AssetOperationJob{Kind: "delete_group", GroupBindingID: &group.ID}

	markAssetOperationDead(&job)
	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	assert.Equal(t, model.AssetBindingStatusActive, group.Status)

	require.NoError(t, model.DB.Model(&group).Update("status", model.AssetBindingStatusDeleting).Error)
	markAssetOperationDead(&job)
	require.NoError(t, model.DB.First(&group, "id = ?", group.ID).Error)
	assert.Equal(t, model.AssetBindingStatusDeletionFailed, group.Status)
	assert.Equal(t, "delete_exhausted", group.ErrorCode)
}

func TestAutomaticAssetGroupPoisonPreflightRejectsUnresolvedCreate(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		errorCode string
		wantError bool
	}{
		{name: "pending", status: model.AssetBindingStatusPending, wantError: true},
		{name: "create unknown", status: model.AssetBindingStatusCreateUnknown, wantError: true},
		{name: "failed unknown", status: model.AssetBindingStatusFailed, errorCode: assetGroupCreationOutcomeUnknownCode, wantError: true},
		{name: "definitive failure", status: model.AssetBindingStatusFailed},
		{name: "active", status: model.AssetBindingStatusActive},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			userID := 950 + index
			seedUser(t, userID, 0)
			asset := model.Asset{UserID: userID, AssetKind: model.AssetKindGeneral}
			binding := model.AssetBinding{ChannelID: 10, CredentialFingerprint: "credential"}
			group := model.AssetGroupBinding{
				UserID: userID, ScopeKey: model.AssetScopeKey(userID, nil), ChannelID: binding.ChannelID,
				CredentialFingerprint: binding.CredentialFingerprint, GroupKind: "general_aigc",
				Status: tt.status, ErrorCode: tt.errorCode,
			}
			require.NoError(t, model.DB.Create(&group).Error)
			adapter := assetadapter.NewJoyCreatorAdapter("https://provider.example", "key", nil)

			err := preflightAutomaticAssetGroupCreate(&asset, &binding, adapter)
			if tt.wantError {
				require.ErrorIs(t, err, ErrAssetUpstreamUnavailable)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
