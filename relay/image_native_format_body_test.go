package relay

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeImageFormatBodyPreservesMediaAndReplay(t *testing.T) {
	for _, multipartBody := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "multipart"}[multipartBody], func(t *testing.T) {
			var source bytes.Buffer
			contentType := "application/json"
			if multipartBody {
				writer := multipart.NewWriter(&source)
				require.NoError(t, writer.WriteField("response_format", "url"))
				for _, image := range []string{"first", "second"} {
					part, err := writer.CreateFormFile("image[]", image+".png")
					require.NoError(t, err)
					_, err = io.WriteString(part, image)
					require.NoError(t, err)
				}
				contentType = writer.FormDataContentType()
				require.NoError(t, writer.Close())
			} else {
				source.WriteString(`{"response_format":"url","image":"data:image/png;base64,Zmlyc3Q=","n":0,"extra":{"response_format":"preserve"}}`)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
			c.Request.Header.Set("Content-Type", contentType)
			info := &relaycommon.RelayInfo{Request: &dto.ImageRequest{ResponseFormat: "url"}, ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI, UpstreamModelName: "gpt-image-1"}}
			body, closer, err := prepareNativeImageFormatBody(c, info, &source)
			require.NoError(t, err)
			require.NotNil(t, closer)
			defer closer.Close()
			data, err := io.ReadAll(body)
			require.NoError(t, err)
			replay, err := body.(common.ReplayableBody).NewReader()
			require.NoError(t, err)
			repeated, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.NoError(t, replay.Close())
			assert.Equal(t, data, repeated)
			if multipartBody {
				assert.NotContains(t, string(data), `name="response_format"`)
				assert.Contains(t, string(data), "first.png")
				assert.Contains(t, string(data), "second.png")
			} else {
				assert.JSONEq(t, `{"image":"data:image/png;base64,Zmlyc3Q=","n":0,"extra":{"response_format":"preserve"}}`, string(data))
			}
			filename := closer.(nativeImageFormatFile).Name()
			require.NoError(t, closer.Close())
			_, err = os.Stat(filename)
			assert.ErrorIs(t, err, os.ErrNotExist)
			_, err = body.(common.ReplayableBody).NewReader()
			assert.Error(t, err)
		})
	}
}

func TestImageSpoolReserveFailsWithoutWriting(t *testing.T) {
	t.Setenv("IMAGE_DELIVERY_MIN_FREE_MB", "1099511627775")
	file, err := os.CreateTemp(t.TempDir(), "spool-*")
	require.NoError(t, err)
	defer file.Close()
	n, err := io.Copy(imageDeliverySpoolWriter{file}, strings.NewReader("media"))
	assert.Zero(t, n)
	assert.ErrorIs(t, err, errImageDeliveryDiskReserve)
	stat, err := file.Stat()
	require.NoError(t, err)
	assert.Zero(t, stat.Size())
}
