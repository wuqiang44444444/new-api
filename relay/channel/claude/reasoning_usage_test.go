package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeChatCompletionsReasoningUsage(t *testing.T) {
	const inputUsage = `"input_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":10}`
	const outputUsage = `"output_tokens":50`
	const thinking30 = `"output_tokens_details":{"thinking_tokens":30}`
	const thinking0 = `"output_tokens_details":{"thinking_tokens":0}`

	for _, tc := range []struct {
		name         string
		startDetails string
		endDetails   string
		wantThinking int
	}{
		{name: "reported in final usage", endDetails: "," + thinking30, wantThinking: 30},
		{name: "no breakdown"},
		{name: "partial final usage omits earlier breakdown", startDetails: "," + thinking30, endDetails: `,"output_tokens_details":{}`, wantThinking: 30},
		{name: "explicit zero replaces earlier breakdown", startDetails: "," + thinking30, endDetails: "," + thinking0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"json", "sse"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "json" && tc.startDetails != "" {
						t.Skip("only streaming has cumulative usage events")
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					info := &relaycommon.RelayInfo{
						RelayFormat:        types.RelayFormatOpenAI,
						ShouldIncludeUsage: true,
						ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-4-6"},
					}
					state := &ClaudeResponseInfo{Usage: &dto.Usage{}}
					var displayUsage *dto.Usage
					if mode == "json" {
						body := `{"id":"msg_test","type":"message","model":"claude-sonnet-4-6","stop_reason":"end_turn","content":[{"type":"thinking","thinking":"summary"},{"type":"text","text":"answer"}],"usage":{` + inputUsage + "," + outputUsage + tc.endDetails + `}}`
						require.Nil(t, HandleClaudeResponseData(c, info, state, nil, []byte(body)))
						var response dto.OpenAITextResponse
						require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
						displayUsage = &response.Usage
					} else {
						for _, event := range []string{
							`{"type":"message_start","message":{"id":"msg_test","model":"claude-sonnet-4-6","usage":{` + inputUsage + tc.startDetails + `}}}`,
							`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"summary"}}`,
							`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}`,
							`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{` + outputUsage + tc.endDetails + `}}`,
							`{"type":"message_stop"}`,
						} {
							require.Nil(t, HandleStreamResponseData(c, info, state, event))
						}
						HandleStreamFinalResponse(c, info, state)
						for _, line := range strings.Split(recorder.Body.String(), "\n") {
							data, ok := strings.CutPrefix(line, "data: ")
							if !ok || data == "[DONE]" {
								continue
							}
							var chunk dto.ChatCompletionsStreamResponse
							require.NoError(t, common.UnmarshalJsonStr(data, &chunk))
							if chunk.Usage != nil {
								displayUsage = chunk.Usage
							}
						}
						assert.Contains(t, recorder.Body.String(), "data: [DONE]")
					}
					require.NotNil(t, displayUsage)
					assert.Equal(t, tc.wantThinking, displayUsage.CompletionTokenDetails.ReasoningTokens)
					assert.Equal(t, 130, displayUsage.PromptTokens)
					assert.Equal(t, 50, displayUsage.CompletionTokens)
					assert.Equal(t, 180, displayUsage.TotalTokens)

					wantBilling := dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{
						InputTokens:              100,
						OutputTokens:             50,
						CacheReadInputTokens:     20,
						CacheCreationInputTokens: 10,
						CacheCreation:            &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: 10},
					})
					expected, ok := wantBilling.CanonicalUsage()
					require.True(t, ok)
					billable, ok := state.Usage.BillingUsage.CanonicalUsage()
					require.True(t, ok)
					assert.Equal(t, expected, billable)
					assert.Equal(t, wantBilling, displayUsage.BillingUsage)
				})
			}
		})
	}
}
