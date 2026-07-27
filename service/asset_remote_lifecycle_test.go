package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveUnknownRemoteCreateExpiresUnresolvedStateAtomically(t *testing.T) {
	truncate(t)
	seedUser(t, 933, 0)
	asset := model.Asset{UserID: 933, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusCreateUnknown}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", Status: model.AssetBindingStatusCreateUnknown}
	require.NoError(t, model.DB.Create(&binding).Error)
	idempotency := model.AssetCreateIdempotency{UserID: asset.UserID, Endpoint: "/v1/assets", KeyHash: "unknown-key", RequestHMAC: "request", AssetID: asset.ID, Status: model.AssetCreateIdempotencyCreateUnknown, ExpiresAt: common.GetTimestamp() + 3600}
	require.NoError(t, model.DB.Create(&idempotency).Error)
	job := model.AssetOperationJob{IdempotencyKey: "resolve-unknown-create-test", Kind: "resolve_unknown_create", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30}
	require.NoError(t, model.DB.Create(&job).Error)

	require.NoError(t, resolveUnknownRemoteCreate(&job))
	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	require.NoError(t, model.DB.First(&idempotency, "id = ?", idempotency.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetStatusFailed, asset.Status)
	assert.Equal(t, model.AssetBindingStatusFailed, binding.Status)
	assert.Equal(t, model.AssetCreateIdempotencyFailed, idempotency.Status)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}

func TestResolveUnknownRemoteCreateOnlyFinishesWatchdogForResolvedAsset(t *testing.T) {
	truncate(t)
	seedUser(t, 934, 0)
	asset := model.Asset{UserID: 934, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusReady}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", UpstreamResourceID: "resource", Status: model.AssetBindingStatusActive}
	require.NoError(t, model.DB.Create(&binding).Error)
	idempotency := model.AssetCreateIdempotency{UserID: asset.UserID, Endpoint: "/v1/assets", KeyHash: "resolved-key", RequestHMAC: "request", AssetID: asset.ID, Status: model.AssetCreateIdempotencyComplete, ExpiresAt: common.GetTimestamp() + 3600}
	require.NoError(t, model.DB.Create(&idempotency).Error)
	job := model.AssetOperationJob{IdempotencyKey: "resolve-complete-create-test", Kind: "resolve_unknown_create", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30}
	require.NoError(t, model.DB.Create(&job).Error)

	require.NoError(t, resolveUnknownRemoteCreate(&job))
	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	require.NoError(t, model.DB.First(&idempotency, "id = ?", idempotency.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetStatusReady, asset.Status)
	assert.Equal(t, model.AssetBindingStatusActive, binding.Status)
	assert.Equal(t, model.AssetCreateIdempotencyComplete, idempotency.Status)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}
