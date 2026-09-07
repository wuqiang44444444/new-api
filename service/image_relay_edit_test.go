package service

import (
	"bytes"
	"encoding/base64"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"image"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestImageRelayInputValidationAfterRequestChange(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	request := &dto.ImageRequest{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "edit"}
	for _, width := range []int{15, 14, 15} {
		var b bytes.Buffer
		require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, 15))))
		var err error
		request.Image, err = common.Marshal("data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes()))
		require.NoError(t, err)
		_, apiErr := ParseImageRelayContract(c, info, request, dto.ImageUpstreamProtocolMoxingImagesV1)
		if width == 14 {
			require.NotNil(t, apiErr)
		} else {
			require.Nil(t, apiErr)
		}
	}
	info.RelayMode = relayconstant.RelayModeImagesGenerations
	_, apiErr := ParseImageRelayContract(c, info, request, dto.ImageUpstreamProtocolMoxingImagesV1)
	require.NotNil(t, apiErr, "cached edit cannot authorize generation inputs")
	info.RelayMode = relayconstant.RelayModeImagesEdits
	request.Image = nil
	urls := make([]string, 11)
	for i := range urls {
		urls[i] = "https://example.com/ref.png"
	}
	var err error
	request.Images, err = common.Marshal(urls)
	require.NoError(t, err)
	_, apiErr = ParseImageRelayContract(c, info, request, dto.ImageUpstreamProtocolMoxingImagesV1)
	require.Nil(t, apiErr)
	request.Model = constant.MoxingImageProviderModelSeedream5Pro
	_, apiErr = ParseImageRelayContract(c, info, request, dto.ImageUpstreamProtocolMoxingImagesV1)
	require.NotNil(t, apiErr, "model changes must revalidate count")
}

func TestImageRelayRejectsInvalidBytes(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{
		{"invalid content", "image/png", []byte("broken")},
		{"unsupported format", "image/gif", []byte("GIF89a")},
		{"provider byte limit", "image/png", make([]byte, 10_000_001)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := common.Marshal("data:" + tc.mime + ";base64," + base64.StdEncoding.EncodeToString(tc.data))
			require.NoError(t, err)
			_, apiErr := ParseImageRelayContract(nil, info, &dto.ImageRequest{Model: constant.FunCloudImageProviderModelSeedream5Lite, Prompt: "edit", Image: raw}, dto.ImageUpstreamProtocolFunCloudAIGCV2)
			assert.NotNil(t, apiErr)
		})
	}
}
