package advancedcustom

import (
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicImageSKUsUseUnifiedNorthboundPublicSubset(t *testing.T) {
	const requestTemplate = `{
		"model":"PUBLIC_MODEL",
		"prompt":"use the subject from the reference image",
		"image":["https://cdn.example/reference.png"],
		"size":"2K",
		"n":1,
		"response_format":"url",
		"stream":false
	}`

	tests := []struct {
		name               string
		publicModel        string
		upstreamModel      string
		upstreamPath       string
		converter          string
		wantImageField     bool
		wantReferenceField bool
	}{
		{
			name:           "Moxing Seedream",
			publicModel:    "seedream-5-moxing",
			upstreamModel:  "seedream-5-0-260128",
			upstreamPath:   "/v1/images/generations",
			converter:      dto.AdvancedCustomConverterMediaTaskImageBlocking,
			wantImageField: true,
		},
		{
			name:           "Qihang Seedream",
			publicModel:    "seedream-5-qihang",
			upstreamModel:  "seedream-5",
			upstreamPath:   "/v1/images/generations",
			converter:      relayconvert.ConverterNone,
			wantImageField: true,
		},
		{
			name:               "Moxing Nano Banana 2",
			publicModel:        "nano-banana-2",
			upstreamModel:      "gemini-3.1-flash-image-preview-usage",
			upstreamPath:       "/v1/media/generations",
			converter:          dto.AdvancedCustomConverterMediaTaskImageBlocking,
			wantReferenceField: true,
		},
	}

	var northboundGolden string
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request dto.ImageRequest
			requestJSON := strings.Replace(requestTemplate, "PUBLIC_MODEL", test.publicModel, 1)
			require.NoError(t, common.Unmarshal([]byte(requestJSON), &request))
			assert.Empty(t, request.Extra)

			northbound := request
			northbound.Model = ""
			northboundJSON, err := common.Marshal(northbound)
			require.NoError(t, err)
			if northboundGolden == "" {
				northboundGolden = string(northboundJSON)
			} else {
				assert.JSONEq(t, northboundGolden, string(northboundJSON))
			}

			request.SetModelName(test.upstreamModel)
			config := &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/images/generations",
					UpstreamPath: test.upstreamPath,
					Converter:    test.converter,
					Models:       []string{test.publicModel},
				},
			}}
			info := advancedCustomRelayInfo(config)
			info.RelayFormat = types.RelayFormatOpenAIImage
			info.RelayMode = relayconstant.RelayModeImagesGenerations
			info.RequestURLPath = "/v1/images/generations"
			info.OriginModelName = test.publicModel
			info.UpstreamModelName = test.upstreamModel
			c := advancedCustomGinContext("/v1/images/generations")

			adaptor := &Adaptor{}
			converted, err := adaptor.ConvertImageRequest(c, info, request)
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)

			var upstreamFields map[string]any
			require.NoError(t, common.Unmarshal(body, &upstreamFields))
			assert.Equal(t, test.upstreamModel, upstreamFields["model"])
			assert.Equal(t, test.wantImageField, upstreamFields["image"] != nil)
			assert.Equal(t, test.wantReferenceField, upstreamFields["reference_images"] != nil)

			requestURL, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			parsedURL, err := url.Parse(requestURL)
			require.NoError(t, err)
			assert.Equal(t, test.upstreamPath, parsedURL.Path)
		})
	}
}

func TestUnifiedImageFieldPreservesModelSpecificCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		publicModel   string
		upstreamModel string
		size          string
		wantError     string
	}{
		{
			name:          "Seedream accepts its 3K capability",
			publicModel:   "seedream-5-moxing",
			upstreamModel: "seedream-5-0-260128",
			size:          "3K",
		},
		{
			name:          "Nano accepts its 4K capability",
			publicModel:   "nano-banana-2",
			upstreamModel: "gemini-3.1-flash-image-preview-usage",
			size:          "4K",
		},
		{
			name:          "Seedream rejects Nano-only 4K value",
			publicModel:   "seedream-5-moxing",
			upstreamModel: "seedream-5-0-260128",
			size:          "4K",
			wantError:     "size must be 2K or 3K",
		},
		{
			name:          "Nano rejects Seedream-only 3K value",
			publicModel:   "nano-banana-2",
			upstreamModel: "gemini-3.1-flash-image-preview-usage",
			size:          "3K",
			wantError:     "size must be one of 1K, 2K or 4K",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := dto.ImageRequest{
				Model:  test.upstreamModel,
				Prompt: "restyle the reference",
				N:      uintPointer(1),
				Size:   test.size,
				Image:  rawJSON(t, "https://cdn.example/reference.png"),
			}

			converted, err := convertMediaTaskImageRequest(request, test.publicModel)
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)
				return
			}

			require.NoError(t, err)
			if test.publicModel == "nano-banana-2" {
				assert.Empty(t, converted.Image)
				assert.Equal(t, []string{"https://cdn.example/reference.png"}, converted.ReferenceImages)
				return
			}
			assert.JSONEq(t, `"https://cdn.example/reference.png"`, string(converted.Image))
			assert.Empty(t, converted.ReferenceImages)
		})
	}
}

func TestUnifiedImageValidationErrorsHideSouthboundContract(t *testing.T) {
	t.Run("Nano reports the public image field", func(t *testing.T) {
		_, err := convertMediaTaskImageRequest(dto.ImageRequest{
			Model:  "gemini-3.1-flash-image-preview-usage",
			Prompt: "combine references",
			N:      uintPointer(1),
			Size:   "1K",
			Image:  rawJSON(t, repeatImageURLs(mediaTaskImageMaxGeminiImages+1)),
		}, "nano-banana-2")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "image must not contain more than")
		assert.NotContains(t, err.Error(), "reference_images")
		assert.NotContains(t, err.Error(), "gemini-3.1-flash-image-preview-usage")
	})

	t.Run("Seedream hides the mapped upstream model ID", func(t *testing.T) {
		_, err := convertMediaTaskImageRequest(dto.ImageRequest{
			Model:  "seedream-5-0-260128",
			Prompt: "generate an image",
			N:      uintPointer(1),
			Size:   "4K",
		}, "seedream-5-moxing")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "size must be 2K or 3K")
		assert.NotContains(t, err.Error(), "seedream-5-0-260128")
	})

	t.Run("errors hide the internal converter", func(t *testing.T) {
		stream := true
		_, err := convertMediaTaskImageRequest(dto.ImageRequest{
			Model:  "gemini-3.1-flash-image-preview-usage",
			Prompt: "generate an image",
			N:      uintPointer(1),
			Size:   "1K",
			Stream: &stream,
		}, "nano-banana-2")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "streaming image responses are not supported")
		assert.NotContains(t, err.Error(), dto.AdvancedCustomConverterMediaTaskImageBlocking)
	})
}
