package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelDefaultAssetGroupUpsertAndChannelDelete(t *testing.T) {
	db := withSeedanceChannelDB(t)
	channel := seedanceTestChannel("seedance-default-group", common.ChannelStatusEnabled)
	require.NoError(t, db.Create(channel).Error)

	require.NoError(t, SaveChannelDefaultAssetGroup(channel.Id, "provider-group-one"))
	require.NoError(t, SaveChannelDefaultAssetGroup(channel.Id, "provider-group-two"))
	record, err := GetChannelDefaultAssetGroup(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "provider-group-two", record.ProviderGroupID)

	require.NoError(t, deleteChannel(channel))
	record, err = GetChannelDefaultAssetGroup(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, record)
}
