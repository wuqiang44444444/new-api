package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTImage2ChatCompletionsRequestContractUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const requestJSON = `{
		"model":"gpt-image-2",
		"messages":[{"role":"user","content":"draw a production image"}],
		"stream":false
	}`

	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(requestJSON), &request))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	require.Equal(t, relayconstant.RelayModeChatCompletions, relayMode)

	info := &relaycommon.RelayInfo{
		RelayMode:       relayMode,
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "gpt-image-2",
		RequestURLPath:  "/v1/chat/completions",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    "https://upstream.example",
			UpstreamModelName: "gpt-image-2",
		},
	}
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertOpenAIRequest(c, info, &request)
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, requestJSON, string(body))

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example/v1/chat/completions", requestURL)
	assert.NotEqual(t, "https://upstream.example/v1/images/generations", requestURL)
}
