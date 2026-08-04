package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestInitTaskFreezesAssetReferencesPrivately(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId: 7,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeDoubaoVideo,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			ClientProtocol:  TaskClientProtocolModelArkV3,
			AssetPublicIDs:  []string{"ast_one", "ast_two"},
			AssetBindingIDs: []int64{21, 22},
		},
	}
	task := InitTask(constant.TaskPlatform("doubao"), info)
	assert.Equal(t, []string{"ast_one", "ast_two"}, task.PrivateData.AssetPublicIDs)
	assert.Equal(t, []int64{21, 22}, task.PrivateData.AssetBindingIDs)
}
