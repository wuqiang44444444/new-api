package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiChatMixedStreamWritesSummaryAndAnswer(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	const thoughtOnly = `data: {"candidates":[{"content":{"parts":[{"thought":true,"text":"previous "}]}}]}` + "\n\n"
	const mixed = `data: {"candidates":[{"content":{"parts":[{"thought":true,"text":"summary"},{"text":"answer"}]},"finishReason":"STOP",` +
		`"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://example.com","title":"Example"}}],"groundingSupports":[{"segment":{"partIndex":1,"startIndex":0,"endIndex":6,"text":"answer"},"groundingChunkIndices":[0]}]}}],` +
		`"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}` + "\n\n"
	for _, test := range []struct {
		name            string
		upstream        string
		force           bool
		thinking        bool
		wantMixedAnswer string
		wantAllAnswer   string
	}{
		{name: "default", upstream: mixed, wantMixedAnswer: "answer", wantAllAnswer: "answer"},
		{name: "force format", upstream: mixed, force: true, wantMixedAnswer: "answer", wantAllAnswer: "answer"},
		{name: "thinking in content", upstream: mixed, thinking: true,
			wantMixedAnswer: "<think>\nsummary\n</think>\nanswer", wantAllAnswer: "<think>\nsummary\n</think>\nanswer"},
		{name: "close already open thinking", upstream: thoughtOnly + mixed, thinking: true,
			wantMixedAnswer: "summary\n</think>\nanswer", wantAllAnswer: "<think>\nprevious summary\n</think>\nanswer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				info := &relaycommon.RelayInfo{
					RelayFormat:        types.RelayFormatOpenAI,
					ShouldIncludeUsage: true,
					DisablePing:        true,
					ChannelMeta: &relaycommon.ChannelMeta{
						UpstreamModelName: "gemini-fixture",
						ChannelSetting: dto.ChannelSettings{
							ForceFormat:       test.force,
							ThinkingToContent: test.thinking,
						},
					},
				}
				resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.upstream))}
				usage, apiErr := gemini.GeminiChatStreamHandler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 3, usage.TotalTokens)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(chatSSERecorder{recorder}, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			require.Equal(t, http.StatusOK, recorder.Code)

			var answer strings.Builder
			var mixedFrames, stopFrames int
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				event, ok := strings.CutPrefix(line, "data: ")
				if !ok || event == "[DONE]" {
					continue
				}
				for _, choice := range gjson.Get(event, "choices").Array() {
					delta := choice.Get("delta")
					thought := delta.Get("reasoning_content")
					require.True(t, thought.Exists(), event)
					content := delta.Get("content").Str
					answer.WriteString(content)
					if strings.Contains(content, "answer") {
						mixedFrames++
						assert.Equal(t, test.wantMixedAnswer, content)
						citation := delta.Get("annotations.0.url_citation")
						require.True(t, citation.Exists(), event)
						runes := []rune(content)
						assert.Equal(t, "answer", string(runes[citation.Get("start_index").Int():citation.Get("end_index").Int()]))
						if test.thinking {
							assert.Equal(t, "null", thought.Raw)
						} else {
							assert.Equal(t, "summary", thought.Str)
						}
					}
					if choice.Get("finish_reason").Str == "stop" {
						stopFrames++
					}
				}
			}
			assert.Equal(t, 1, mixedFrames)
			assert.Equal(t, 1, stopFrames)
			assert.Equal(t, test.wantAllAnswer, answer.String())
			assert.Contains(t, recorder.Body.String(), "data: [DONE]\n\n")
		})
	}
}
