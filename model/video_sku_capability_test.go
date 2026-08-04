package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeicaiVideoSKUCapabilitiesAreStableAndResolutionBound(t *testing.T) {
	models := map[string]string{
		VideoSKUSeedance20Standard720P: "720p",
		VideoSKUSeedance20Value720P:    "720p",
	}
	for publicModel, resolution := range models {
		t.Run(publicModel, func(t *testing.T) {
			first, ok := ResolveVideoSKUCapability(publicModel)
			require.True(t, ok)
			second, ok := ResolveVideoSKUCapability(publicModel)
			require.True(t, ok)
			assert.Equal(t, resolution, first.Resolution)
			assert.Equal(t, first.ContentHash, second.ContentHash)
			assert.Len(t, first.ContentHash, 64)
			assert.True(t, first.SupportsProfile(VideoProfileJSONMediaArrays))
			assert.Equal(t, 4, first.DefaultDuration)
			assert.Equal(t, []string{"16:9", "9:16"}, first.Ratios)
			assert.Equal(t, []string{"reference_image"}, first.ImageRoles)
			assert.Empty(t, first.VideoRoles)
			assert.Equal(t, []string{"reference_audio"}, first.AudioRoles)
			assert.True(t, first.SupportsLinkAssets)
			assert.False(t, first.SupportsMixedMediaPath)
			assert.Equal(t, 0, first.MaxVideos)

			wrong := "720p"
			if resolution == wrong {
				wrong = "1080p"
			}
			err := first.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
				Model:      publicModel,
				Resolution: common.GetPointer(wrong),
				Content: []dto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("hello")},
				},
			})
			require.Error(t, err)

			err = first.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
				Model: publicModel,
				Content: []dto.ModelArkVideoContent{
					{
						Type:     "image_url",
						Role:     common.GetPointer("reference_image"),
						ImageURL: &dto.VideoMediaURL{URL: "https://example.com/reference.png"},
					},
				},
			})
			require.ErrorContains(t, err, "text")

			err = first.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
				Model:       publicModel,
				ServiceTier: common.GetPointer("default"),
				Content: []dto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("hello")},
				},
			})
			require.ErrorContains(t, err, "service_tier")

			err = first.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
				Model: publicModel,
				Content: []dto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("hello")},
					{
						Type:     "image_url",
						Role:     common.GetPointer("first_frame"),
						ImageURL: &dto.VideoMediaURL{URL: "https://example.com/first.png"},
					},
					{
						Type:     "audio_url",
						Role:     common.GetPointer("reference_audio"),
						AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"},
					},
				},
			})
			require.ErrorContains(t, err, "first_frame")
		})
	}
}

func TestFeicaiHighResolutionSKUsRemainUnpublishedWithoutVerifiedSize(t *testing.T) {
	for _, publicModel := range []string{
		VideoSKUSeedance20Standard1080P,
		VideoSKUSeedance20Value1080P,
		VideoSKUSeedance20Value4K,
	} {
		_, ok := ResolveVideoSKUCapability(publicModel)
		assert.False(t, ok, publicModel)
	}
}

func TestVideoSKUCapabilityLimitsAreDataDriven(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	capability.MaxImages = 1

	err := capability.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
		Model: capability.PublicModel,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("hello")},
			{
				Type:     "image_url",
				Role:     common.GetPointer("reference_image"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/one.png"},
			},
			{
				Type:     "image_url",
				Role:     common.GetPointer("reference_image"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/two.png"},
			},
		},
	})

	require.ErrorContains(t, err, "maximum of 1")
}

func TestFunCloudVideoSKUCapabilitiesMatchTheirEndpoints(t *testing.T) {
	standard, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"480p", "720p", "1080p"}, standard.Resolutions)
	assert.True(t, standard.SupportsLinkAssets)
	assert.False(t, standard.AllowsAutomaticDuration)
	assert.True(t, standard.SupportsProfile(VideoProfileFunCloudSeedanceV2))

	fast, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Fast)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"480p", "720p"}, fast.Resolutions)
	assert.True(t, fast.SupportsLinkAssets)
	assert.False(t, fast.AllowsAutomaticDuration)

	duration := -1
	request := &dto.ModelArkVideoCreateRequest{
		Model:    VideoSKUSeedance20Standard,
		Duration: &duration,
		Content:  []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
	}
	require.ErrorContains(t, standard.ValidateModelArkRequest(request), "duration")

}

func TestKlingAndJimengCapabilitiesOwnPublishedRequestValidation(t *testing.T) {
	kling, ok := ResolveVideoSKUCapability(VideoSKUKlingV1)
	require.True(t, ok)
	err := kling.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName:   common.GetPointer(VideoSKUKlingV1),
			Prompt:      common.GetPointer("move"),
			Duration:    common.GetPointer("15"),
			AspectRatio: common.GetPointer("16:9"),
		},
	})
	require.ErrorContains(t, err, "duration")

	err = kling.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName: common.GetPointer(VideoSKUKlingV1),
			Prompt:    common.GetPointer("move"),
			Duration:  common.GetPointer("10"),
		},
	})
	require.NoError(t, err)

	jimeng, ok := ResolveVideoSKUCapability(VideoSKUJimengVGFMT2VL20)
	require.True(t, ok)
	err = jimeng.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractJimeng,
		Jimeng:     &dto.JimengVideoCreateRequest{ReqKey: "unknown"},
	})
	require.ErrorContains(t, err, "does not match")
	require.NoError(t, jimeng.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractJimeng,
		Jimeng:     &dto.JimengVideoCreateRequest{ReqKey: VideoSKUJimengVGFMT2VL20},
	}))
}
