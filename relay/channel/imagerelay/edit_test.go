package imagerelay

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRelayEditContracts(t *testing.T) {
	for _, protocol := range []dto.ImageUpstreamProtocol{dto.ImageUpstreamProtocolFunCloudAIGCV2, dto.ImageUpstreamProtocolMoxingImagesV1} {
		for _, model := range constant.ImageRelayProviderModels(protocol) {
			t.Run(model, func(t *testing.T) {
				info := imageRelayTestInfo(protocol)
				info.UpstreamModelName = model
				info.RelayMode = relayconstant.RelayModeImagesEdits
				req := dto.ImageRequest{Model: model, Prompt: "change cup color", Images: []byte(`["https://example.com/one.png","https://example.com/two.png"]`)}
				converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, req)
				require.NoError(t, err)
				require.NoError(t, service.PrepareImageUpstreamRequest(t.Context(), converted))
				body, err := common.Marshal(converted)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, common.Unmarshal(body, &payload))
				key := "image"
				if protocol == dto.ImageUpstreamProtocolFunCloudAIGCV2 {
					key = "imageUrls"
				} else {
					assert.Equal(t, "image_generation", payload["capability"])
				}
				assert.Equal(t, []any{"https://example.com/one.png", "https://example.com/two.png"}, payload[key])
				if model == constant.FunCloudImageProviderModelSeedream5Lite || model == constant.FunCloudImageProviderModelSeedream5Pro {
					assert.Equal(t, "i2i", payload["genType"])
				}
				assert.NotContains(t, payload, "reference_images")
				limit, _, _ := constant.ImageRelayInputLimits(protocol, model)
				urls := make([]string, limit+1)
				for i := range urls {
					urls[i] = "https://example.com/ref.png"
				}
				req.Images, err = common.Marshal(urls)
				require.NoError(t, err)
				_, err = (&Adaptor{}).ConvertImageRequest(nil, info, req)
				require.ErrorContains(t, err, "at most")
				req.Images = nil
				_, err = (&Adaptor{}).ConvertImageRequest(nil, info, req)
				require.ErrorContains(t, err, "at least")
				req.Images = []byte(`["https://example.com/ref.png"]`)
				req.N = common.GetPointer(uint(0))
				_, err = (&Adaptor{}).ConvertImageRequest(nil, info, req)
				require.Error(t, err)
				req.N = nil
				info.RelayMode = relayconstant.RelayModeImagesGenerations
				_, err = (&Adaptor{}).ConvertImageRequest(nil, info, req)
				require.ErrorContains(t, err, "only accepted")
			})
		}
	}
}

func TestImageRelayMultipartAndJSONEditEquivalent(t *testing.T) {
	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(img.Bytes())
	for _, protocol := range []dto.ImageUpstreamProtocol{dto.ImageUpstreamProtocolFunCloudAIGCV2, dto.ImageUpstreamProtocolMoxingImagesV1} {
		t.Run(string(protocol), func(t *testing.T) {
			info := imageRelayTestInfo(protocol)
			info.RelayMode = relayconstant.RelayModeImagesEdits
			if protocol == dto.ImageUpstreamProtocolMoxingImagesV1 {
				info.UpstreamModelName = constant.MoxingImageProviderModelSeedream5Lite
			}
			req := dto.ImageRequest{Model: info.UpstreamModelName, Prompt: "change cup color"}
			var err error
			req.Image, err = common.Marshal(dataURL)
			require.NoError(t, err)
			jsonRequest, err := (&Adaptor{}).ConvertImageRequest(nil, info, req)
			require.NoError(t, err, "conversion must not require storage")
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("image", "ref.png")
			require.NoError(t, err)
			_, err = part.Write(img.Bytes())
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/edits", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			require.NoError(t, c.Request.ParseMultipartForm(1<<20))
			t.Cleanup(func() { _ = c.Request.MultipartForm.RemoveAll() })
			req.Image = nil
			formRequest, err := (&Adaptor{}).ConvertImageRequest(c, info, req)
			require.NoError(t, err)
			assert.Equal(t, jsonRequest, formRequest)
			if protocol == dto.ImageUpstreamProtocolMoxingImagesV1 {
				encoded, err := common.Marshal(formRequest)
				require.NoError(t, err)
				var p map[string]any
				require.NoError(t, common.Unmarshal(encoded, &p))
				assert.Equal(t, dataURL, p["image"])
			}
		})
	}
}
