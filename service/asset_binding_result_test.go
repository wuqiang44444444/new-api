package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollingDoesNotEraseKnownUpstreamAssetIdentity(t *testing.T) {
	truncate(t)
	seedUser(t, 932, 0)
	asset := model.Asset{UserID: 932, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusProcessing}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{
		AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets",
		UpstreamResourceID: "resource-1", UpstreamBusinessID: "business-1", UpstreamRequestID: "request-1",
		UpstreamReferenceType: "asset_uri_id", UpstreamReferenceValue: "asset-1", Status: model.AssetBindingStatusProcessing,
	}
	require.NoError(t, model.DB.Create(&binding).Error)
	job := model.AssetOperationJob{IdempotencyKey: "poll-binding-preserve-id", Kind: "poll_binding", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30}
	require.NoError(t, model.DB.Create(&job).Error)

	err := saveAssetBindingResult(&job, &asset, &binding, assetadapter.AssetResult{Status: "processing", RequestID: "request-2"})
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	assert.Equal(t, "resource-1", binding.UpstreamResourceID)
	assert.Equal(t, "business-1", binding.UpstreamBusinessID)
	assert.Equal(t, "asset_uri_id", binding.UpstreamReferenceType)
	assert.Equal(t, "asset-1", binding.UpstreamReferenceValue)
	assert.Equal(t, "request-2", binding.UpstreamRequestID)
}

func TestLatePollResultDoesNotReviveDeletingAsset(t *testing.T) {
	truncate(t)
	seedUser(t, 937, 0)
	asset := model.Asset{UserID: 937, Name: "remote", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusDeleting}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{
		AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets",
		UpstreamResourceID: "resource-1", UpstreamReferenceType: "asset_uri_id", UpstreamReferenceValue: "asset-1", Status: model.AssetBindingStatusDeleting,
	}
	require.NoError(t, model.DB.Create(&binding).Error)
	job := model.AssetOperationJob{IdempotencyKey: "poll-binding-deleting", Kind: "poll_binding", AssetID: &asset.ID, BindingID: &binding.ID, Status: model.AssetJobRunning, LockedBy: "runner", LockedUntil: 1 << 30}
	require.NoError(t, model.DB.Create(&job).Error)

	require.NoError(t, saveAssetBindingResult(&job, &asset, &binding, assetadapter.AssetResult{Status: "active", ReferenceType: "asset_uri_id", ReferenceValue: "asset-1"}))
	require.NoError(t, model.DB.First(&asset, "id = ?", asset.ID).Error)
	require.NoError(t, model.DB.First(&binding, "id = ?", binding.ID).Error)
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, model.AssetStatusDeleting, asset.Status)
	assert.Equal(t, model.AssetBindingStatusDeleting, binding.Status)
	assert.Equal(t, model.AssetJobSucceeded, job.Status)
}
