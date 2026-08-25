package publicmodel

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parameterByName(t *testing.T, parameters []dto.PublicAPIParameter, name string) dto.PublicAPIParameter {
	t.Helper()
	for _, parameter := range parameters {
		if parameter.Name == name {
			return parameter
		}
	}
	require.FailNow(t, "parameter not found", name)
	return dto.PublicAPIParameter{}
}

func assertParameterAbsent(t *testing.T, parameters []dto.PublicAPIParameter, name string) {
	t.Helper()
	for _, parameter := range parameters {
		assert.NotEqual(t, name, parameter.Name)
	}
}

func parameterNames(parameters []dto.PublicAPIParameter) []string {
	names := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		names = append(names, parameter.Name)
	}
	return names
}

func TestImageAPIPublishesStrictPerModelFields(t *testing.T) {
	lite, ok := ImageAPI("nano-banana-2-lite", dto.ImageUpstreamProtocolFunCloudAIGCV2, constant.FunCloudImageProviderModelNanoBanana2Lite)
	require.True(t, ok)
	require.NotNil(t, lite.Image)
	assert.False(t, lite.Image.Creation.AdditionalProperties)
	assertParameterAbsent(t, lite.Image.Creation.Parameters, "size")
	assertParameterAbsent(t, lite.Image.Creation.Parameters, "watermark")
	assert.Equal(t, 1, parameterByName(t, lite.Image.Creation.Parameters, "n").FixedValue)

	seedreamPro, ok := ImageAPI("seedream-5.0-pro", dto.ImageUpstreamProtocolFunCloudAIGCV2, constant.FunCloudImageProviderModelSeedream5Pro)
	require.True(t, ok)
	assert.Equal(t, []string{"1K"}, parameterByName(t, seedreamPro.Image.Creation.Parameters, "size").Enum)
	assertParameterAbsent(t, seedreamPro.Image.Creation.Parameters, "watermark")

	mappedPro, ok := ImageAPI("seedream-5-pro-moxing", dto.ImageUpstreamProtocolMoxingImagesV1, constant.MoxingImageProviderModelSeedream5Pro)
	require.True(t, ok)
	assert.Equal(t, []string{"2K"}, parameterByName(t, mappedPro.Image.Creation.Parameters, "size").Enum)
	assertParameterAbsent(t, mappedPro.Image.Creation.Parameters, "extra_fields")
}

func TestAllRegisteredImageProfilesPublishParameters(t *testing.T) {
	tests := []struct {
		protocol dto.ImageUpstreamProtocol
		model    string
	}{
		{dto.ImageUpstreamProtocolFunCloudAIGCV2, constant.FunCloudImageProviderModelNanoBanana2Lite},
		{dto.ImageUpstreamProtocolFunCloudAIGCV2, constant.FunCloudImageProviderModelNanoBanana2},
		{dto.ImageUpstreamProtocolFunCloudAIGCV2, constant.FunCloudImageProviderModelSeedream5Lite},
		{dto.ImageUpstreamProtocolFunCloudAIGCV2, constant.FunCloudImageProviderModelSeedream5Pro},
		{dto.ImageUpstreamProtocolMoxingImagesV1, constant.MoxingImageProviderModelSeedream5Lite},
		{dto.ImageUpstreamProtocolMoxingImagesV1, constant.MoxingImageProviderModelSeedream5Pro},
	}
	for _, test := range tests {
		api, ok := ImageAPI("customer-image", test.protocol, test.model)
		require.True(t, ok)
		require.NotNil(t, api.Image)
		assert.NotEmpty(t, api.Image.Creation.Parameters)
		assert.Equal(t, "customer-image", api.Image.Creation.Model)
	}
}

func TestVideoAPIPublishesMappedModelConstraintsWithoutPrivateIdentity(t *testing.T) {
	api, ok := VideoAPI("public-video", dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-vip-4k-azhw", false)
	require.True(t, ok)
	assert.Equal(t, "public-video", api.Creation.Model)
	assert.False(t, api.Creation.AdditionalProperties)
	assert.Equal(t, []string{"4k"}, parameterByName(t, api.Creation.Parameters, "resolution").Enum)
	assert.True(t, parameterByName(t, api.Creation.Parameters, "duration").Required)
	assertParameterAbsent(t, api.Creation.Parameters, "watermark")
	assertParameterAbsent(t, api.Creation.Parameters, "service_tier")

	encoded, err := common.Marshal(api)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "feicai")
	assert.NotContains(t, string(encoded), "seedance-2.0-vip-4k-azhw")
}

func TestAllRegisteredVideoProfilesPublishParameters(t *testing.T) {
	tests := []struct {
		protocol dto.VideoUpstreamProtocol
		model    string
	}{
		{dto.VideoUpstreamProtocolModelArkV3Volcengine, "doubao-seedance-2-0-260128"},
		{dto.VideoUpstreamProtocolModelArkV3Volcengine, "doubao-seedance-2-0-fast-260128"},
		{dto.VideoUpstreamProtocolModelArkV3Volcengine, "doubao-seedance-2-0-mini-260615"},
		{dto.VideoUpstreamProtocolModelArkV3Volcengine, "doubao-seedance-2-5-260628"},
		{dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, "doubao-seedance-2-0-260128"},
		{dto.VideoUpstreamProtocolMoxingMediaTaskV1, "doubao-seedance-2-0-260128"},
		{dto.VideoUpstreamProtocolMoxingModelArkV1, "doubao-seedance-2-0-fast-260128"},
		{dto.VideoUpstreamProtocolMoxingModelArkV1, "doubao-seedance-2-0-mini-260615"},
		{dto.VideoUpstreamProtocolMoxingModelArkV1, "doubao-seedance-2-5-260628"},
		{dto.VideoUpstreamProtocolFunCloudSeedance, "seedance-2"},
		{dto.VideoUpstreamProtocolFunCloudSeedance, "seedance-2-fast"},
		{dto.VideoUpstreamProtocolFunCloudSeedance, "seedance-2-mini"},
		{dto.VideoUpstreamProtocolFunCloudSeedance, "seedance-2-5"},
		{dto.VideoUpstreamProtocolModelArkV3CMCC, "doubao-seedance-2.0"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-vip-720p-mini-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance2.0-sd2"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-vip-720p-fast-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-933-720p-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-vip-720p-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-933-1080p-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-vip-1080p-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-933-4k-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-2.0-vip-4k-azhw"},
		{dto.VideoUpstreamProtocolFeicaiVideosV1, "seedance-933-pro-pi"},
	}
	for _, test := range tests {
		api, ok := VideoAPI("customer-video", test.protocol, test.model, false)
		require.True(t, ok)
		require.NotNil(t, api)
		assert.NotEmpty(t, api.Creation.Parameters)
		assert.NotEmpty(t, api.Creation.ContentTypes)
		assert.Equal(t, "customer-video", api.Creation.Model)
	}
}

func TestUnregisteredVideoProfilesDoNotPublishFallbackContract(t *testing.T) {
	tests := []dto.VideoUpstreamProtocol{
		dto.VideoUpstreamProtocolModelArkV3CMCC,
		dto.VideoUpstreamProtocolTokenSaveMediaTaskV1,
		dto.VideoUpstreamProtocolMoxingMediaTaskV1,
		dto.VideoUpstreamProtocolMoxingModelArkV1,
		dto.VideoUpstreamProtocolFunCloudSeedance,
		dto.VideoUpstreamProtocolFeicaiVideosV1,
	}
	for _, protocol := range tests {
		api, ok := VideoAPI("customer-video", protocol, "unregistered-provider-model", false)
		assert.False(t, ok, protocol)
		assert.Nil(t, api, protocol)
	}
}

func TestNativeVideoProfilePublishesOpenAIVideosParameters(t *testing.T) {
	api := NativeVideoAPI("sora-2-pro")
	require.NotNil(t, api.Video)
	assert.Equal(t, "openai_videos", api.Video.Protocol)
	assert.Equal(t, "/v1/videos", api.Video.Creation.Path)
	assert.Equal(t,
		[]string{"720x1280", "1280x720", "1792x1024", "1024x1792"},
		parameterByName(t, api.Video.Creation.Parameters, "size").Enum,
	)
	names := parameterNames(api.Video.Creation.Parameters)
	assert.Contains(t, names, "input_reference")
	assert.NotContains(t, names, "duration")
	assert.NotContains(t, names, "metadata")
	assert.Equal(t, []string{"4", "8", "12"}, parameterByName(t, api.Video.Creation.Parameters, "seconds").Enum)
}

func TestNativeImageAPIPublishesModelSpecificGenerationParameters(t *testing.T) {
	t.Run("gpt image excludes edit and legacy response fields", func(t *testing.T) {
		api := NativeImageAPI("gpt-image-2")
		require.NotNil(t, api.Image)
		names := parameterNames(api.Image.Creation.Parameters)
		assert.Contains(t, names, "output_compression")
		assert.Contains(t, names, "partial_images")
		assert.NotContains(t, names, "response_format")
		assert.NotContains(t, names, "images")
		assert.NotContains(t, names, "watermark")
		assert.Empty(t, parameterByName(t, api.Image.Creation.Parameters, "size").Enum)
	})

	t.Run("dall e 3 fixes image count", func(t *testing.T) {
		api := NativeImageAPI("dall-e-3")
		require.NotNil(t, api.Image)
		n := parameterByName(t, api.Image.Creation.Parameters, "n")
		assert.Equal(t, 1, n.FixedValue)
		assert.Equal(t, []string{"standard", "hd"}, parameterByName(t, api.Image.Creation.Parameters, "quality").Enum)
		assert.NotContains(t, parameterNames(api.Image.Creation.Parameters), "background")
	})
}
