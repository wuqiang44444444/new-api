package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type chatSSERecorder struct {
	*httptest.ResponseRecorder
}

func (chatSSERecorder) CloseNotify() <-chan bool { return nil }

func TestChatCompletionReasoningChannelWriters(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	const simpleChat = `{"id":"fixture","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	const simpleDelta = "data: " + `{"id":"fixture","choices":[{"delta":{"content":"answer"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
	const geminiAnswer = `{"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
	const responsesStream = "data: " + `{"type":"response.reasoning_summary_text.delta","delta":"summary"}` + "\n\ndata: " +
		`{"type":"response.output_text.delta","delta":"answer"}` + "\n\ndata: " +
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}` + "\n\n"

	for _, test := range []struct {
		name        string
		apiType     int
		apiKey      string
		stream      bool
		body        string
		wantThought string
		wantStopID  string
		wantDone    bool
		convert     func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "Baidu JSON", apiType: constant.APITypeBaidu, body: `{"id":"fixture","result":"answer","usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`},
		{name: "Baidu SSE", apiType: constant.APITypeBaidu, stream: true, body: "data: " + `{"result":"answer","is_end":true,"usage":{"prompt_tokens":2,"total_tokens":3}}` + "\n\n"},
		{name: "Cohere JSON", apiType: constant.APITypeCohere, body: `{"text":"answer","meta":{"billed_units":{"input_tokens":2,"output_tokens":1}}}`},
		{name: "Cohere SSE", apiType: constant.APITypeCohere, stream: true, wantDone: true, body: `{"text":"answer"}
{"is_finished":true,"finish_reason":"COMPLETE"}
`},
		{name: "Cloudflare JSON", apiType: constant.APITypeCloudflare, body: simpleChat},
		{name: "Cloudflare SSE", apiType: constant.APITypeCloudflare, stream: true, wantDone: true, body: simpleDelta},
		{name: "Coze JSON", apiType: constant.APITypeCoze, body: `{"code":0,"data":[{"type":"answer","content":"answer"}]}`},
		{name: "Coze SSE", apiType: constant.APITypeCoze, stream: true, wantDone: true, body: `event: conversation.message.delta
data: {"content":"answer"}

event: conversation.chat.completed
data: {"usage":{"input_count":2,"output_count":1,"token_count":3}}

`},
		{name: "Dify JSON", apiType: constant.APITypeDify, body: `{"answer":"answer"}`},
		{name: "Dify SSE", apiType: constant.APITypeDify, stream: true, wantDone: true, body: "data: " + `{"event":"message","answer":"answer"}` + "\n\ndata: " + `{"event":"message_end"}` + "\n\n"},
		{name: "PaLM JSON", apiType: constant.APITypePaLM, body: `{"candidates":[{"content":"answer"}]}`},
		{name: "PaLM SSE", apiType: constant.APITypePaLM, stream: true, wantDone: true, body: `{"candidates":[{"content":"answer"}]}`},
		{name: "Zhipu JSON", apiType: constant.APITypeZhipu, body: `{"success":true,"data":{"choices":[{"role":"assistant","content":"answer"}]}}`},
		{name: "Zhipu SSE with meta", apiType: constant.APITypeZhipu, stream: true, wantDone: true, wantStopID: "meta-fixture", body: `data:answer
meta:{"request_id":"meta-fixture","usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}
`},
		{name: "Tencent JSON", apiType: constant.APITypeTencent, apiKey: "fixture|fixture", body: `{"Response":{"Choices":[{"Message":{"Role":"assistant","Content":"answer"},"FinishReason":"stop"}],"Usage":{"PromptTokens":2,"CompletionTokens":1,"TotalTokens":3}}}`},
		{name: "Tencent SSE", apiType: constant.APITypeTencent, apiKey: "fixture|fixture", stream: true, wantDone: true, body: "data: " + `{"Choices":[{"Delta":{"Content":"answer"},"FinishReason":"stop"}]}` + "\n\n"},
		{name: "xAI JSON", apiType: constant.APITypeXai, body: simpleChat},
		{name: "xAI SSE", apiType: constant.APITypeXai, stream: true, wantDone: true, body: simpleDelta},
		{name: "Gemini JSON without thought", apiType: constant.APITypeGemini, body: geminiAnswer},
		{name: "Gemini JSON with multiple thoughts", apiType: constant.APITypeGemini, wantThought: "first second", body: `{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"first "},{"text":"answer"},{"thought":true,"text":"second"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`},
		{name: "Gemini SSE", apiType: constant.APITypeGemini, stream: true, wantDone: true, wantThought: "summary", body: "data: " +
			`{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"summary"}]}}]}` + "\n\ndata: " + geminiAnswer + "\n\n"},
		{name: "Claude JSON", apiType: constant.APITypeAnthropic, wantThought: "summary", body: `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"summary"},{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`},
		{name: "Claude SSE", apiType: constant.APITypeAnthropic, stream: true, wantDone: true, wantThought: "summary", body: `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"fixture","usage":{"input_tokens":2,"output_tokens":0}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"summary"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}

data: {"type":"content_block_stop","index":1}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

data: {"type":"message_stop"}

`},
		{name: "Ollama JSON", apiType: constant.APITypeOllama, wantThought: "summary", body: `{"model":"fixture","message":{"role":"assistant","content":"answer","thinking":"summary"},"done":true,"prompt_eval_count":2,"eval_count":1}`},
		{name: "Ollama NDJSON to SSE", apiType: constant.APITypeOllama, stream: true, wantDone: true, wantThought: "summary", body: `{"model":"fixture","message":{"role":"assistant","thinking":"summary"},"done":false}
{"model":"fixture","message":{"role":"assistant","content":"answer"},"done":false}
{"model":"fixture","done":true,"prompt_eval_count":2,"eval_count":1}
`},
		{name: "Responses JSON to Chat", convert: openai.OaiResponsesToChatHandler, wantThought: "summary", body: `{"id":"resp_1","status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"summary"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`},
		{name: "Responses SSE to Chat SSE", convert: openai.OaiResponsesToChatStreamHandler, stream: true, wantDone: true, wantThought: "summary", body: responsesStream},
		{name: "Responses SSE to buffered Chat JSON", convert: openai.OaiResponsesToChatBufferedStreamHandler, wantThought: "summary", body: responsesStream},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				info := &relaycommon.RelayInfo{
					RelayMode:   relayconstant.RelayModeChatCompletions,
					RelayFormat: types.RelayFormatOpenAI,
					IsStream:    test.stream,
					DisablePing: true,
					StartTime:   time.Now(),
					ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fixture", ApiKey: test.apiKey},
				}
				resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}
				var apiErr *types.NewAPIError
				if test.convert != nil {
					_, apiErr = test.convert(c, info, resp)
				} else {
					adaptor := GetAdaptor(test.apiType)
					adaptor.Init(info)
					_, apiErr = adaptor.DoResponse(c, resp, info)
				}
				require.Nil(t, apiErr)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(chatSSERecorder{recorder}, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			require.Equal(t, http.StatusOK, recorder.Code)

			key := "message"
			payloads := []string{recorder.Body.String()}
			if test.stream {
				key = "delta"
				payloads = nil
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					data, ok := strings.CutPrefix(line, "data:")
					if ok && strings.TrimSpace(data) != "[DONE]" {
						payloads = append(payloads, strings.TrimSpace(data))
					}
				}
				assert.Equal(t, test.wantDone, strings.Contains(recorder.Body.String(), "data: [DONE]"))
			}

			var content, thought strings.Builder
			var sawStopID, sawNull bool
			for _, payload := range payloads {
				require.True(t, gjson.Valid(payload), payload)
				for _, choice := range gjson.Get(payload, "choices").Array() {
					message := choice.Get(key)
					require.True(t, message.IsObject(), payload)
					reasoning := message.Get("reasoning_content")
					require.True(t, reasoning.Exists(), payload)
					switch reasoning.Type {
					case gjson.String:
						thought.WriteString(reasoning.Str)
					case gjson.Null:
						sawNull = true
					default:
						t.Fatalf("unexpected reasoning_content: %s", reasoning.Raw)
					}
					content.WriteString(message.Get("content").Str)
					if test.wantStopID != "" && gjson.Get(payload, "id").Str == test.wantStopID {
						assert.Equal(t, "stop", choice.Get("finish_reason").Str)
						assert.Equal(t, "null", reasoning.Raw)
						sawStopID = true
					}
				}
			}
			assert.Equal(t, "answer", content.String())
			if test.wantThought != "" {
				assert.Equal(t, test.wantThought, thought.String())
			} else {
				assert.Empty(t, thought.String())
				assert.True(t, sawNull)
			}
			if test.wantStopID != "" {
				assert.True(t, sawStopID)
			}
		})
	}
}
