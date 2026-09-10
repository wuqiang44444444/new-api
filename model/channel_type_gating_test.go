package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedLocalChannelsSkipGenericAbilityPool(t *testing.T) {
	assert.True(t, channelSkipsGenericAbilities(constant.ChannelTypeSeedanceLink))
	assert.True(t, channelSkipsGenericAbilities(constant.ChannelTypeAzureBatch),
		"batch channels must never enter the generic sync dispatch pool")
	assert.False(t, channelSkipsGenericAbilities(constant.ChannelTypeAzure))
	assert.False(t, channelSkipsGenericAbilities(constant.ChannelTypeOpenAI))
}

func TestSelectEnabledBatchChannelIsDeterministicAndGroupScoped(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	first := Channel{Name: "batch-1", Type: constant.ChannelTypeAzureBatch, Group: "batch-group", Models: "batch-model", Key: "k", Status: common.ChannelStatusEnabled, Other: "2026-09-01-preview"}
	require.NoError(t, db.Create(&first).Error)
	second := Channel{Name: "batch-2", Type: constant.ChannelTypeAzureBatch, Group: "batch-group", Models: "batch-model", Key: "k", Status: common.ChannelStatusEnabled, Other: "2026-09-01-preview"}
	require.NoError(t, db.Create(&second).Error)
	otherGroup := Channel{Name: "batch-other", Type: constant.ChannelTypeAzureBatch, Group: "default", Models: "batch-model", Key: "k", Status: common.ChannelStatusEnabled, Other: "2026-09-01-preview"}
	require.NoError(t, db.Create(&otherGroup).Error)
	disabled := Channel{Name: "batch-off", Type: constant.ChannelTypeAzureBatch, Group: "batch-group", Models: "batch-model", Key: "k", Status: 3, Other: "2026-09-01-preview"}
	require.NoError(t, db.Create(&disabled).Error)

	selected, err := SelectEnabledBatchChannel("batch-group", "batch-model")
	require.NoError(t, err)
	assert.Equal(t, first.Id, selected.Id, "the lowest enabled channel id wins deterministically")
	_, err = SelectEnabledBatchChannel("missing-group", "batch-model")
	assert.Error(t, err)
}
