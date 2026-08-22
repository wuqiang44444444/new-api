package imagerelay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imageRelayTestInfo(protocol dto.ImageUpstreamProtocol) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl:    "https://provider.example",
		UpstreamModelName: constant.FunCloudImageProviderModelNanoBanana2,
		ChannelOtherSettings: dto.ChannelOtherSettings{
			ImageUpstreamProtocol: protocol,
		},
	}}
}

func TestImageRelayAdaptorDispatchesOnlyByExplicitProtocol(t *testing.T) {
	funCloudURL, err := (&Adaptor{}).GetRequestURL(imageRelayTestInfo(dto.ImageUpstreamProtocolFunCloudAIGCV2))
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/api/v2/open/aigc/nano-banana-2", funCloudURL)

	moxingInfo := imageRelayTestInfo(dto.ImageUpstreamProtocolMoxingImagesV1)
	moxingInfo.UpstreamModelName = constant.MoxingImageProviderModelSeedream5Lite
	moxingURL, err := (&Adaptor{}).GetRequestURL(moxingInfo)
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/v1/images/generations", moxingURL)

	_, err = (&Adaptor{}).GetRequestURL(imageRelayTestInfo(""))
	require.ErrorContains(t, err, "image_upstream_protocol")
}

func TestImageRelayAdaptorPublishesUnionWithoutInferringProtocol(t *testing.T) {
	models := (&Adaptor{}).GetModelList()
	assert.Contains(t, models, constant.FunCloudImageProviderModelNanoBanana2)
	assert.Contains(t, models, constant.MoxingImageProviderModelSeedream5Lite)
	assert.Equal(t, "Image Relay", (&Adaptor{}).GetChannelName())
}
