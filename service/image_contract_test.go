package service

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

// 最小 1x1 PNG，用于构造合法 data URL 与 multipart 输入。
var testPNGBytes = mustBase64Decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

func mustBase64Decode(value string) []byte {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return data
}

func imageDataURL() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNGBytes)
}

func contractRequest(body string) *dto.ImageRequest {
	request := &dto.ImageRequest{}
	if err := commonUnmarshalForTest([]byte(body), request); err != nil {
		panic(err)
	}
	return request
}

func TestParseImageContractGenerationsDefaults(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	request := contractRequest(`{"model":"nano-banana-2-gemini","prompt":"a cat"}`)
	contract, apiErr := ParseImageContract(nil, info, request)
	require.Nil(t, apiErr)
	assert.Equal(t, ImageOperationGenerations, contract.Operation)
	assert.Equal(t, uint(1), contract.N)
	assert.Equal(t, "", contract.ResponseFormat)
	assert.Empty(t, contract.Images)
}

func TestParseImageContractNZeroAndOutOfRangeRejected(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	// 显式 0 不是默认值（E6/P3）。
	request := contractRequest(`{"model":"m","prompt":"p","n":0}`)
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)

	request = contractRequest(fmt.Sprintf(`{"model":"m","prompt":"p","n":%d`, MaxUnifiedImageN+1) + `}`)
	_, apiErr = ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
}

func TestParseImageContractEditsImages(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	request := contractRequest(fmt.Sprintf(`{"model":"m","prompt":"p","images":["%s","https://example.com/a.png"]}`, imageDataURL()))
	contract, apiErr := ParseImageContract(nil, info, request)
	require.Nil(t, apiErr)
	require.Len(t, contract.Images, 2)
	assert.Equal(t, "image/png", contract.Images[0].MimeType)
	assert.Equal(t, testPNGBytes, contract.Images[0].Data)
	assert.True(t, contract.Images[1].IsURL())
	assert.Equal(t, "https://example.com/a.png", contract.Images[1].URL)
}

func TestParseImageContractEditsRequireImages(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	request := contractRequest(`{"model":"m","prompt":"p"}`)
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "at least one input image")
}

func TestParseImageContractGenerationsRejectsImages(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	request := contractRequest(fmt.Sprintf(`{"model":"m","prompt":"p","images":["%s"]}`, imageDataURL()))
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "only accepted by /v1/images/edits")
}

func TestParseImageContractRejectsMaskEverywhere(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	request := contractRequest(`{"model":"m","prompt":"p","image":"https://example.com/a.png","mask":"https://example.com/m.png"}`)
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "mask is not supported")
}

func TestParseImageContractRejectsHTTPUrlInputs(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	request := contractRequest(`{"model":"m","prompt":"p","image":"http://example.com/a.png"}`)
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "HTTPS")
}

func TestParseImageContractInputCountBudget(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	items := ""
	for i := 0; i < MaxImageInputs+1; i++ {
		if i > 0 {
			items += ","
		}
		items += `"https://example.com/a.png"`
	}
	request := contractRequest(fmt.Sprintf(`{"model":"m","prompt":"p","images":[%s]}`, items))
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "at most")
}

func TestParseImageContractImageAndImagesMutuallyExclusive(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	request := contractRequest(fmt.Sprintf(`{"model":"m","prompt":"p","image":"%s","images":["https://example.com/a.png"]}`, imageDataURL()))
	_, apiErr := ParseImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "must not be combined")
}

func TestRejectNonEmptyImageFieldsTreatsEmptyAsUnset(t *testing.T) {
	request := contractRequest(`{"model":"m","prompt":"p","quality":"","style":null,"output_format":""}`)
	assert.Nil(t, RejectNonEmptyImageFields(request, "quality", "style", "output_format"))

	request = contractRequest(`{"model":"m","prompt":"p","quality":"high"}`)
	apiErr := RejectNonEmptyImageFields(request, "quality")
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "quality")
}

func TestRejectExtraJSONFields(t *testing.T) {
	request := contractRequest(`{"model":"m","prompt":"p","callbackUrl":"https://x"}`)
	apiErr := RejectExtraJSONFields(request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "callbackUrl")
}

func TestPreferRespondAsyncParsing(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"respond-async", true},
		{"Respond-Async", true},
		{"respond-async, wait=60", true},
		{"wait=60, respond-async", true},
		{"wait", false},
		{"respond-async-max-wait=30", false},
	}
	for _, tc := range tests {
		c := &gin.Context{}
		c.Request, _ = http.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		c.Request.Header.Set("Prefer", tc.header)
		assert.Equal(t, tc.want, PreferRespondAsync(c), "Prefer: %q", tc.header)
	}
}

func commonUnmarshalForTest(data []byte, v any) error {
	return common.Unmarshal(data, v)
}
