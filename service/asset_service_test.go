package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenameAssetCreatesAJobForEveryAcceptedRename(t *testing.T) {
	truncate(t)
	seedUser(t, 938, 0)
	asset := model.Asset{UserID: 938, Name: "first", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusReady}
	require.NoError(t, model.DB.Create(&asset).Error)
	binding := model.AssetBinding{AssetID: asset.ID, UserID: asset.UserID, ChannelID: 1, CredentialFingerprint: "credential", UpstreamProfile: "relay_assets", UpstreamResourceID: "resource", Status: model.AssetBindingStatusActive}
	require.NoError(t, model.DB.Create(&binding).Error)

	_, err := RenameAssetForApp(asset.UserID, asset.AppID, asset.PublicID, "second")
	require.NoError(t, err)
	var firstJob model.AssetOperationJob
	require.NoError(t, model.DB.First(&firstJob, "kind = ? AND binding_id = ?", "update_binding", binding.ID).Error)
	require.NoError(t, model.DB.Model(&firstJob).Update("status", model.AssetJobSucceeded).Error)

	_, err = RenameAssetForApp(asset.UserID, asset.AppID, asset.PublicID, "third")
	require.NoError(t, err)
	var jobs []model.AssetOperationJob
	require.NoError(t, model.DB.Where("kind = ? AND binding_id = ?", "update_binding", binding.ID).Order("id asc").Find(&jobs).Error)
	require.Len(t, jobs, 2)
	assert.NotEqual(t, jobs[0].IdempotencyKey, jobs[1].IdempotencyKey)
}

func TestRenameAssetWrapsInvalidName(t *testing.T) {
	truncate(t)
	seedUser(t, 939, 0)
	asset := model.Asset{UserID: 939, Name: "first", AssetKind: model.AssetKindGeneral, MediaType: "image", Status: model.AssetStatusReady}
	require.NoError(t, model.DB.Create(&asset).Error)

	_, err := RenameAssetForApp(asset.UserID, asset.AppID, asset.PublicID, "   ")
	assert.ErrorIs(t, err, ErrInvalidAssetRequest)
}
