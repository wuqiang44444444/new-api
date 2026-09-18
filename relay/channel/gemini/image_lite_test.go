package gemini_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteRespectsAdministratorImagineModelSelection(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	previous := settings.SupportedImagineModels
	t.Cleanup(func() { settings.SupportedImagineModels = previous })
	settings.SupportedImagineModels = []string{"gemini-3.1-flash-image"}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-lite-image"}}
	request := dto.ImageRequest{Model: info.UpstreamModelName, Prompt: "cup", Size: "1024x1024"}
	converted, err := (&gemini.Adaptor{}).ConvertImageRequest(nil, info, request)
	require.Error(t, err)
	assert.Nil(t, converted, "size policy must not grant eligibility outside administrator registration")
	settings.SupportedImagineModels = append(settings.SupportedImagineModels, info.UpstreamModelName)
	converted, err = (&gemini.Adaptor{}).ConvertImageRequest(nil, info, request)
	require.NoError(t, err)
	assert.NotNil(t, converted)
}

func TestLiteStandardImageConversion(t *testing.T) {
	for _, provider := range []string{"gemini", "vertex"} {
		for _, edit := range []bool{false, true} {
			for _, size := range []string{"auto", "1024x1024", "1920x1080", "3840x2160"} {
				t.Run(provider+"/"+map[bool]string{false: "generations", true: "edits"}[edit]+"/"+size, func(t *testing.T) {
					info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations,
						ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-lite-image"}}
					info.ChannelType = map[string]int{"gemini": 24, "vertex": 41}[provider]
					request := dto.ImageRequest{Model: "customer-alias", Prompt: "edit cup", Size: size}
					if edit {
						info.RelayMode = relayconstant.RelayModeImagesEdits
						require.NoError(t, common.Unmarshal([]byte(`{"model":"customer-alias","prompt":"edit cup","images":["https://example.com/image.png"]}`), &request))
						request.Size = size
					}
					var value any
					var err error
					if provider == "gemini" {
						value, err = (&gemini.Adaptor{}).ConvertImageRequest(nil, info, request)
					} else {
						value, err = (&vertex.Adaptor{}).ConvertImageRequest(nil, info, request)
					}
					if size == "1920x1080" || size == "3840x2160" {
						require.Error(t, err)
						var apiErr *types.NewAPIError
						require.ErrorAs(t, err, &apiErr)
						assert.Equal(t, 400, apiErr.StatusCode)
						assert.Nil(t, value)
						return
					}
					require.NoError(t, err)
					converted, ok := value.(*dto.GeminiChatRequest)
					require.True(t, ok)
					assert.Equal(t, []string{"TEXT", "IMAGE"}, converted.GenerationConfig.ResponseModalities)
					if size == "auto" {
						assert.Empty(t, converted.GenerationConfig.ImageConfig)
					} else {
						var config map[string]string
						require.NoError(t, common.Unmarshal(converted.GenerationConfig.ImageConfig, &config))
						assert.Equal(t, map[string]string{"aspectRatio": "1:1", "imageSize": "1K"}, config)
					}
					if edit {
						require.Len(t, converted.Contents[0].Parts, 2)
						assert.Equal(t, "https://example.com/image.png", converted.Contents[0].Parts[1].FileData.FileUri)
					}
				})
			}
		}
	}
}
