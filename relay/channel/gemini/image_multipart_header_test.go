package gemini_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleMultipartEditUsesJSONUpstreamHeader(t *testing.T) {
	for _, provider := range []string{"gemini", "vertex"} {
		t.Run(provider, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("image", "input.png")
			require.NoError(t, err)
			_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
			originalType := writer.FormDataContentType()
			c.Request.Header.Set("Content-Type", originalType)
			require.NoError(t, c.Request.ParseMultipartForm(1<<20))
			t.Cleanup(func() { _ = c.Request.MultipartForm.RemoveAll() })
			info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesEdits, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image", ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey}}}
			request := dto.ImageRequest{Model: "gemini-3.1-flash-image", Prompt: "Make it green"}
			var adaptor interface {
				ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error)
				SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error
			}
			if provider == "gemini" {
				adaptor = &gemini.Adaptor{}
			} else {
				adaptor = &vertex.Adaptor{}
			}
			headers := http.Header{}
			require.NoError(t, adaptor.SetupRequestHeader(c, &headers, info))
			assert.Equal(t, originalType, headers.Get("Content-Type"), "unconverted requests preserve existing behavior")
			converted, err := adaptor.ConvertImageRequest(c, info, request)
			require.NoError(t, err)
			payload, err := common.Marshal(converted)
			require.NoError(t, err)
			assert.Contains(t, string(payload), "inlineData")
			require.NoError(t, adaptor.SetupRequestHeader(c, &headers, info))
			assert.Equal(t, "application/json", headers.Get("Content-Type"))
			assert.Equal(t, originalType, c.Request.Header.Get("Content-Type"), "northbound request metadata remains intact")
		})
	}
}
