package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTChatLatestMappedRequestCompatibility(t *testing.T) {
	for _, tc := range []struct {
		origin, upstream string
		converted        bool
	}{
		{origin: "gpt-chat-latest", upstream: "gpt-chat-latest", converted: true},
		{origin: "chat-latest", upstream: "gpt-chat-latest", converted: true},
		{origin: "customer-chat", upstream: "gpt-chat-latest", converted: true},
		{origin: "gpt-chat-latest", upstream: "deployment-a"},
		{origin: "chat-latest", upstream: "chat-latest"},
	} {
		for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAzure} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s_to_%s/channel=%d/stream=%t", tc.origin, tc.upstream, channelType, stream), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					mapping, err := common.Marshal(map[string]string{tc.origin: tc.upstream})
					require.NoError(t, err)
					c.Set("model_mapping", string(mapping))
					info := &relaycommon.RelayInfo{
						OriginModelName: tc.origin,
						ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: channelType, UpstreamModelName: tc.origin},
					}
					request := &dto.GeneralOpenAIRequest{
						Model: tc.origin, Stream: common.GetPointer(stream), MaxTokens: common.GetPointer(uint(16)),
						Temperature: common.GetPointer(0.7), TopP: common.GetPointer(0.9),
						LogProbs: common.GetPointer(true), TopLogProbs: common.GetPointer(2),
						Messages: []dto.Message{{Role: "system", Content: "Be concise"}, {Role: "user", Content: "Hi"}},
					}
					require.NoError(t, helper.ModelMappedHelper(c, info, request))
					converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, request)
					require.NoError(t, err)
					encoded, err := common.Marshal(converted)
					require.NoError(t, err)
					var body map[string]any
					require.NoError(t, common.Unmarshal(encoded, &body))
					assert.Equal(t, tc.upstream, body["model"])
					assert.Equal(t, stream, body["stream"])
					if tc.converted {
						assert.NotContains(t, body, "max_tokens")
						assert.Equal(t, float64(16), body["max_completion_tokens"])
						for _, field := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
							assert.NotContains(t, body, field)
						}
						assert.Equal(t, "developer", request.Messages[0].Role)
					} else {
						assert.Equal(t, float64(16), body["max_tokens"])
						assert.NotContains(t, body, "max_completion_tokens")
						assert.Equal(t, 0.7, body["temperature"])
						assert.Equal(t, 0.9, body["top_p"])
						assert.Equal(t, true, body["logprobs"])
						assert.Equal(t, float64(2), body["top_logprobs"])
						assert.Equal(t, "system", request.Messages[0].Role)
					}
				})
			}
		}
	}
}
