package gemini_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteVertexInlineInputLimit(t *testing.T) {
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 1, 1))))
	for _, channelType := range []int{24, 41} {
		for _, length := range []int{7000000, 7000001} {
			payload := make([]byte, length)
			copy(payload, pngData.Bytes())
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"customer-image","prompt":"cup","image":"data:image/png;base64,`+base64.StdEncoding.EncodeToString(payload)+`"}`), &request))
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType, UpstreamModelName: "gemini-3.1-flash-lite-image"}}

			for _, formInput := range []bool{false, true} {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
				formRequest := request
				if formInput {
					var form bytes.Buffer
					writer := multipart.NewWriter(&form)
					file, err := writer.CreateFormFile("image", "fixture.png")
					require.NoError(t, err)
					_, err = file.Write(payload)
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &form)
					c.Request.Header.Set("Content-Type", writer.FormDataContentType())
					require.NoError(t, c.Request.ParseMultipartForm(10<<20))
					t.Cleanup(func() { _ = c.Request.MultipartForm.RemoveAll() })
					formRequest.Image = nil
				}
				_, err := gemini.ParseGeminiImageContract(c, info, &formRequest)
				if channelType == 41 && length > 7000000 {
					require.NotNil(t, err)
					assert.Equal(t, 400, err.StatusCode)
				} else {
					require.Nil(t, err)
				}
			}
		}
	}
}

func TestLiteMissingUsageCannotDeliverSuccess(t *testing.T) {
	for _, metadata := range []string{"", `,"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1120,"totalTokenCount":1130}`} {
		body := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]` + metadata + `}`
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-lite-image"}}
		usage, err := gemini.GeminiGenerateContentImageHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})
		require.NotNil(t, err)
		assert.Equal(t, 502, err.StatusCode)
		assert.Equal(t, "image_usage_incomplete", string(err.GetErrorCode()))
		assert.True(t, types.IsSkipRetryError(err))
		assert.Nil(t, usage)
		assert.Empty(t, recorder.Body.String())
	}
}
