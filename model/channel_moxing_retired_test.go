package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestMoxingRetiredProtocolCannotBeConfigured(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := seedanceTestChannel("customer", common.ChannelStatusEnabled)
	channel.BaseURL = common.GetPointer("https://moxing.example")
	channel.ModelMapping = common.GetPointer(`{"customer":"doubao-seedance-2-0-260128"}`)
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingMediaTaskV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
	})
	require.ErrorContains(t, channel.ValidateSettings(), "video protocol is retired")
}
