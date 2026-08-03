package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetSourceUsesScopedEncryptionAndRejectsLegacyEnvelope(t *testing.T) {
	truncateTables(t)
	asset := &Asset{UserID: 701, Name: "source", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady}
	require.NoError(t, DB.Create(asset).Error)

	source, err := CreateAssetSourceTx(DB, asset, "https://cdn.example/source.png", common.GetTimestamp()+600)
	require.NoError(t, err)
	assert.NotContains(t, source.EncryptedURL, "cdn.example")

	plaintext, err := DecryptAssetSourceURL(asset, source)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/source.png", plaintext)

	other := *asset
	other.PublicID = "ast_01234567890123456789012345678901"
	_, err = DecryptAssetSourceURL(&other, source)
	assert.Error(t, err)

	legacy, err := common.EncryptShortLivedSecret("https://cdn.example/legacy.png")
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(legacy, "v2."))
	_, err = DecryptAssetSourceURL(asset, &AssetSource{AssetID: asset.ID, EncryptedURL: legacy})
	assert.Error(t, err)
}

func TestSourceOnlyAssetCreationDoesNotCreateBinding(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 702, Username: "source-only"}).Error)
	asset := &Asset{UserID: 702, Name: "source", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady}

	created, binding, replay, err := CreateRemoteAssetWithQuota(
		asset, "https://cdn.example/source.png", 0, nil, nil, 10, 300,
	)
	require.NoError(t, err)
	assert.False(t, replay)
	assert.Nil(t, binding)
	assert.Equal(t, AssetStatusReady, created.Status)

	var bindingCount, sourceCount int64
	require.NoError(t, DB.Model(&AssetBinding{}).Count(&bindingCount).Error)
	require.NoError(t, DB.Model(&AssetSource{}).Where("asset_id = ?", created.ID).Count(&sourceCount).Error)
	assert.Zero(t, bindingCount)
	assert.EqualValues(t, 1, sourceCount)
}
