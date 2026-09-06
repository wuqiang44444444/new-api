package helper

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

// 评审 S8：合同测试必须从真实路由解析器进入，而不是绕过它直接调
// ParseImageContract。显式 n=0 在原生归一后仍必须被合同层拒绝。
func TestImageRouteParserPreservesExplicitNZeroForContract(t *testing.T) {
	c := &gin.Context{}
	request := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader([]byte(`{"model":"m","prompt":"p","n":0}`)))
	request.Header.Set("Content-Type", "application/json")
	c.Request = request

	parsed, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
	require.NoError(t, err)
	// 原生归一：N 已被改为 1（保持其它渠道行为），但显式零值被记录。
	require.NotNil(t, parsed.N)
	assert.Equal(t, uint(1), *parsed.N)
	assert.True(t, parsed.NExplicitZero, "explicit n=0 must survive the native normalizer for the contract layer")

	_, apiErr := service.ParseImageContract(nil, nil, parsed)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "n must be an integer between 1 and")
}

// 评审 S8：multipart 路径必须完整读取标准标量字段，不能静默丢弃。
func TestImageRouteMultipartParserReadsStandardFields(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("model", "nano-banana-2-gemini"))
	require.NoError(t, writer.WriteField("prompt", "edit this"))
	require.NoError(t, writer.WriteField("response_format", "b64_json"))
	require.NoError(t, writer.WriteField("background", "transparent"))
	require.NoError(t, writer.WriteField("output_format", "png"))
	require.NoError(t, writer.WriteField("n", "0"))
	require.NoError(t, writer.WriteField("image", "https://example.com/a.png"))
	require.NoError(t, writer.Close())

	c := &gin.Context{}
	request := httptest.NewRequest("POST", "/v1/images/edits", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = request

	parsed, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	assert.Equal(t, "b64_json", parsed.ResponseFormat, "response_format must not be silently dropped")
	assert.Equal(t, string(parsed.Background), `"transparent"`, "background must reach the contract layer")
	assert.Equal(t, string(parsed.OutputFormat), `"png"`, "output_format must reach the contract layer")
	assert.True(t, parsed.NExplicitZero, "multipart n=0 keeps its explicit-zero semantics")

	// 合同层按三态语义裁决：background 对未发布族显式 400。
	_, apiErr := service.ParseImageContract(c, nil, parsed)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "n must be an integer between 1 and")

	editsInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	var withoutUnsupported dto.ImageRequest
	withoutUnsupported.Model = parsed.Model
	withoutUnsupported.Prompt = parsed.Prompt
	withoutUnsupported.ResponseFormat = parsed.ResponseFormat
	withoutUnsupported.NExplicitZero = false
	n := uint(1)
	withoutUnsupported.N = &n
	withoutUnsupported.Image = parsed.Image
	contract, apiErr := service.ParseImageContract(c, editsInfo, &withoutUnsupported)
	require.Nil(t, apiErr)
	assert.Equal(t, service.ImageOperationEdits, contract.Operation)
	require.Len(t, contract.Images, 1)
	assert.True(t, contract.Images[0].IsURL())
}
