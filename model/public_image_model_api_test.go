package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicImageModelAPIFollowsMappedPreferredChannel(t *testing.T) {
	resetPricingEndpointTestTables(t)
	channel := &Channel{
		Id: 401, Type: constant.ChannelTypeAsyncImage, Key: "image-key", Status: common.ChannelStatusEnabled,
		Name: "image-channel", Group: "default", Models: "public-image",
		ModelMapping: common.GetPointer(`{"public-image":"doubao-seedream-5-0-pro-260628"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "public-image", ChannelId: channel.Id, Enabled: true,
	}).Error)

	apis, err := GetPublicMediaModelAPIs([]string{"public-image"}, []string{"default"})
	require.NoError(t, err)
	require.NotNil(t, apis["public-image"])
	require.NotNil(t, apis["public-image"].Image)
	assert.Equal(t, "/v1/images/generations", apis["public-image"].Image.Creation.Path)

	parameters := apis["public-image"].Image.Creation.Parameters
	var size dto.PublicAPIParameter
	for _, parameter := range parameters {
		assert.NotEqual(t, "watermark", parameter.Name)
		if parameter.Name == "size" {
			size = parameter
		}
	}
	assert.Equal(t, []string{"2K"}, size.Enum)

	pricingAPIs, err := GetPublicMediaModelAPIs([]string{"public-image"}, nil)
	require.NoError(t, err)
	require.NotNil(t, pricingAPIs["public-image"])
	require.NotNil(t, pricingAPIs["public-image"].Image)
}

func TestPublicImageModelAPIOmitsContractWhenSelectableChannelsDisagree(t *testing.T) {
	resetPricingEndpointTestTables(t)
	channels := []Channel{
		{
			Id: 411, Type: constant.ChannelTypeAsyncImage, Key: "funcloud-key", Status: common.ChannelStatusEnabled,
			Name: "funcloud-image", Group: "default", Models: "shared-image",
			ModelMapping: common.GetPointer(`{"shared-image":"nano-banana-2-lite"}`),
		},
		{
			Id: 412, Type: constant.ChannelTypeAsyncImage, Key: "moxing-key", Status: common.ChannelStatusEnabled,
			Name: "moxing-image", Group: "default", Models: "shared-image",
			ModelMapping: common.GetPointer(`{"shared-image":"doubao-seedream-5-0-pro-260628"}`),
		},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2})
	channels[1].SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "default", Model: "shared-image", ChannelId: 411, Enabled: true},
		{Group: "default", Model: "shared-image", ChannelId: 412, Enabled: true},
	}).Error)

	apis, err := GetPublicMediaModelAPIs([]string{"shared-image"}, []string{"default"})
	require.NoError(t, err)
	assert.NotContains(t, apis, "shared-image")
}

func TestPublicImageModelAPIUsesCallerGroupOrderBeforeChannelPriority(t *testing.T) {
	resetPricingEndpointTestTables(t)
	lowPriority := int64(0)
	highPriority := int64(100)
	channels := []Channel{
		{
			Id: 421, Type: constant.ChannelTypeAsyncImage, Key: "first-key", Status: common.ChannelStatusEnabled,
			Name: "first-group", Group: "first", Models: "ordered-image", Priority: &lowPriority,
			ModelMapping: common.GetPointer(`{"ordered-image":"doubao-seedream-5-0-pro-260628"}`),
		},
		{
			Id: 422, Type: constant.ChannelTypeAsyncImage, Key: "second-key", Status: common.ChannelStatusEnabled,
			Name: "second-group", Group: "second", Models: "ordered-image", Priority: &highPriority,
			ModelMapping: common.GetPointer(`{"ordered-image":"seedream-5.0-pro"}`),
		},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
	channels[1].SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2})
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "first", Model: "ordered-image", ChannelId: 421, Enabled: true, Priority: &lowPriority},
		{Group: "second", Model: "ordered-image", ChannelId: 422, Enabled: true, Priority: &highPriority},
	}).Error)

	apis, err := GetPublicMediaModelAPIs([]string{"ordered-image"}, []string{"first", "second"})
	require.NoError(t, err)
	require.NotNil(t, apis["ordered-image"])
	require.NotNil(t, apis["ordered-image"].Image)
	for _, parameter := range apis["ordered-image"].Image.Creation.Parameters {
		if parameter.Name == "size" {
			assert.Equal(t, []string{"2K"}, parameter.Enum)
			return
		}
	}
	require.Fail(t, "size parameter was not published")
}

func TestPublicImageModelAPIPricingProjectionRequiresAgreementAcrossGroups(t *testing.T) {
	resetPricingEndpointTestTables(t)
	channels := []Channel{
		{
			Id: 431, Type: constant.ChannelTypeAsyncImage, Key: "first-key", Status: common.ChannelStatusEnabled,
			Name: "first-group", Group: "first", Models: "priced-image",
			ModelMapping: common.GetPointer(`{"priced-image":"doubao-seedream-5-0-pro-260628"}`),
		},
		{
			Id: 432, Type: constant.ChannelTypeAsyncImage, Key: "second-key", Status: common.ChannelStatusEnabled,
			Name: "second-group", Group: "second", Models: "priced-image",
			ModelMapping: common.GetPointer(`{"priced-image":"seedream-5.0-pro"}`),
		},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
	channels[1].SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2})
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "first", Model: "priced-image", ChannelId: 431, Enabled: true},
		{Group: "second", Model: "priced-image", ChannelId: 432, Enabled: true},
	}).Error)

	apis, err := GetPublicMediaModelAPIs([]string{"priced-image"}, nil)
	require.NoError(t, err)
	assert.NotContains(t, apis, "priced-image")
}
