package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveChannelLinkExecutionsUsesOrdinaryModelMapping(t *testing.T) {
	channel := &Channel{
		Type:         constant.ChannelTypeDoubaoVideo,
		Models:       "customer-seedance",
		ModelMapping: common.GetPointer(`{"customer-seedance":"seedance-2.0-vip-720p-azhw-feicai"}`),
	}
	settings := dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV2,
		},
	}

	executions, err := DeriveChannelLinkExecutions(channel, &settings)
	require.NoError(t, err)
	require.Len(t, executions, 1)
	assert.Equal(t, "customer-seedance", executions[0].CustomerModel)
	assert.Equal(t, "seedance-2.0-vip-720p-azhw-feicai", executions[0].ProviderModel)
	assert.Equal(t, VideoSKUSeedance20Standard720P, executions[0].LinkSKU)
}

func TestDeriveChannelLinkExecutionsFailsClosedWithoutExactBinding(t *testing.T) {
	channel := &Channel{Type: constant.ChannelTypeKling, Models: "customer-kling"}
	settings := dto.ChannelOtherSettings{LinkImplementation: dto.LinkImplementationRef{
		ID: LinkImplementationKlingVideos, Version: LinkImplementationVersionV1,
	}}

	_, err := DeriveChannelLinkExecutions(channel, &settings)
	require.Error(t, err)
}

func TestDeriveChannelLinkExecutionsPreservesMappedModelLiteral(t *testing.T) {
	channel := &Channel{
		Type:         constant.ChannelTypeDoubaoVideo,
		Models:       "customer-seedance",
		ModelMapping: common.GetPointer(`{"customer-seedance":" seedance-2.0-vip-720p-azhw-feicai "}`),
	}
	settings := dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV2,
		},
	}

	_, err := DeriveChannelLinkExecutions(channel, &settings)
	require.ErrorContains(t, err, `Provider model " seedance-2.0-vip-720p-azhw-feicai "`)
}

func TestDeriveChannelLinkExecutionsRejectsAmbiguousAdvancedCustomRoutes(t *testing.T) {
	channel := &Channel{
		Type:         constant.ChannelTypeAdvancedCustom,
		Models:       "customer-image",
		ModelMapping: common.GetPointer(`{"customer-image":"seedream-5-0-260128"}`),
	}
	route := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/images/generations",
		UpstreamPath: "/v1/images/generations",
		Converter:    dto.AdvancedCustomConverterMediaTaskImageBlocking,
		Models:       []string{"customer-image"},
	}
	settings := dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{route, route}},
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationMoxingImages, Version: LinkImplementationVersionV1,
		},
	}

	_, err := DeriveChannelLinkExecutions(channel, &settings)
	require.ErrorContains(t, err, "exactly one image generation route")
}
