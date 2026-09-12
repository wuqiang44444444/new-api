package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedSeedancePromotionIsMonotonicAndPreservesHistory(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.AutoMigrate(&TaskPlugin{}))
	old := configurationTestPlugin("1.9.0", "provider-one")
	next := configurationTestPlugin("1.10.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&old))
	require.NoError(t, SaveTaskPlugin(&next))
	require.NoError(t, PromoteEmbeddedSeedancePlugin(next.Version, next.SourceHash))
	// A previous-release node sharing this database must not downgrade it.
	require.NoError(t, PromoteEmbeddedSeedancePlugin(old.Version, old.SourceHash))
	active, err := GetTaskPluginVersion(old.Key, "")
	require.NoError(t, err)
	assert.Equal(t, next.Version, active.Version)
	assert.True(t, active.Enabled)
	storedOld, err := GetTaskPluginVersion(old.Key, old.Version)
	require.NoError(t, err)
	assert.False(t, storedOld.Active)
	assert.Equal(t, old.Source, storedOld.Source)
	assert.Equal(t, old.SourceHash, storedOld.SourceHash)

	require.NoError(t, SetTaskPluginEnabled(next.Key, false))
	require.NoError(t, PromoteEmbeddedSeedancePlugin(next.Version, next.SourceHash))
	active, err = GetTaskPluginVersion(old.Key, "")
	require.NoError(t, err)
	assert.False(t, active.Enabled, "same-version restarts preserve the administrator's stop switch")
	require.ErrorContains(t, PromoteEmbeddedSeedancePlugin(next.Version, "different-source"), "conflicts with its stored artifact")
}

func TestEmbeddedSeedancePromotionValidatesExistingChannelsBeforeSwitching(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.AutoMigrate(&TaskPlugin{}))
	old := configurationTestPlugin("1.0.0", "provider-one")
	next := configurationTestPlugin("2.0.0", "provider-two")
	require.NoError(t, SaveTaskPlugin(&old))
	require.NoError(t, SaveTaskPlugin(&next))
	channel := seedanceTestChannel("provider-one", common.ChannelStatusEnabled)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	require.NoError(t, db.Create(channel).Error)
	require.ErrorContains(t, PromoteEmbeddedSeedancePlugin(next.Version, next.SourceHash), "incompatible with channels")
	active, err := GetTaskPluginVersion(old.Key, "")
	require.NoError(t, err)
	assert.Equal(t, old.Version, active.Version)
	var kept Channel
	require.NoError(t, db.First(&kept, channel.Id).Error)
	assert.Equal(t, channel.Models, kept.Models)
	assert.Equal(t, channel.GetOtherSettings(), kept.GetOtherSettings())
}
