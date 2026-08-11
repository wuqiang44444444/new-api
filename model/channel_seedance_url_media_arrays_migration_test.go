package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateSeedanceURLMediaArraysProtocolUpdatesOnlyCurrentChannelSettings(t *testing.T) {
	db := withSeedanceChannelDB(t)
	legacy := Channel{
		Type:          constant.ChannelTypeSeedanceLink,
		Name:          "legacy URL media channel",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: `{"video_upstream_protocol":"media_arrays_v2","asset_upstream_protocol":"none"}`,
	}
	official := seedanceTestChannel("official-model", common.ChannelStatusEnabled)
	official.Name = "official channel"
	records := []*Channel{&legacy, official}
	for _, channel := range records {
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(channel).Error)
	}

	require.NoError(t, migrateSeedanceURLMediaArraysProtocol())
	require.NoError(t, migrateSeedanceURLMediaArraysProtocol())

	var migrated Channel
	require.NoError(t, db.First(&migrated, legacy.Id).Error)
	assert.Equal(t, dto.VideoUpstreamProtocolURLMediaArraysV1, migrated.GetOtherSettings().VideoUpstreamProtocol)
	assert.Equal(t, dto.AssetUpstreamProtocolNone, migrated.GetOtherSettings().AssetUpstreamProtocol)

	var unchanged Channel
	require.NoError(t, db.First(&unchanged, official.Id).Error)
	assert.Equal(t, dto.VideoUpstreamProtocolModelArkV3Volcengine, unchanged.GetOtherSettings().VideoUpstreamProtocol)
}
