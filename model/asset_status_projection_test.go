package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetStatusProjectionUsesCurrentResolutionPaths(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	assets := []Asset{
		{UserID: 711, AppID: 71, Name: "valid-source", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady},
		{UserID: 711, AppID: 71, Name: "expired-source", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady},
	}
	require.NoError(t, DB.Create(&assets).Error)
	_, err := CreateAssetSourceTx(DB, &assets[0], "https://cdn.example/valid.png", now+600)
	require.NoError(t, err)
	_, err = CreateAssetSourceTx(DB, &assets[1], "https://cdn.example/expired.png", now-1)
	require.NoError(t, err)

	projected, err := GetAssetByPublicIDForApp(711, 71, assets[1].PublicID)
	require.NoError(t, err)
	require.NotNil(t, projected)
	assert.Equal(t, AssetStatusFailed, projected.Status)
	assert.Equal(t, "asset_source_expired", projected.ErrorCode)

	ready, total, err := ListAssetsByApp(711, 71, 0, 20, AssetListFilter{Status: AssetStatusReady})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, ready, 1)
	assert.Equal(t, assets[0].PublicID, ready[0].PublicID)

	failed, total, err := ListAssetsByApp(711, 71, 0, 20, AssetListFilter{Status: AssetStatusFailed})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, failed, 1)
	assert.Equal(t, assets[1].PublicID, failed[0].PublicID)
}

func TestAssetStatusProjectionKeepsExpiredSourceReadyWithCurrentBinding(t *testing.T) {
	truncateTables(t)
	channel, fingerprint := seedAssetLifecycleChannel(t, 712, "projection-key", string(dto.AssetUpstreamProfileRelay))
	asset := Asset{UserID: 712, AppID: 72, Name: "bound", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady}
	require.NoError(t, DB.Create(&asset).Error)
	_, err := CreateAssetSourceTx(DB, &asset, "https://cdn.example/expired.png", common.GetTimestamp()-1)
	require.NoError(t, err)
	binding := newAssetBindingForTest(t, channel, asset.UserID, fingerprint, string(dto.AssetUpstreamProfileRelay))
	binding.AssetID = asset.ID
	binding.Status = AssetBindingStatusActive
	binding.UpstreamReferenceType = "asset_uri_id"
	binding.UpstreamReferenceValue = "provider-asset-id"
	require.NoError(t, DB.Create(binding).Error)

	projected := []Asset{asset}
	require.NoError(t, ProjectAssetStatuses(projected, common.GetTimestamp()))
	assert.Equal(t, AssetStatusReady, projected[0].Status)
}
