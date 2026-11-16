package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionReasoningOnlyChangesRegisteredResponse(t *testing.T) {
	const input = `{"messages":[{"role":"assistant","content":"earlier","reasoning_content":null}]}`
	const upstream = `{"model":"fixture","choices":[{"message":{"role":"assistant","content":"answer","reasoning":"step","private_extension":{"number":9007199254740993}},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	const extended = `{"model":"fixture","choices":[{"message":{"role":"assistant","content":"answer","reasoning":"step","private_extension":{"number":9007199254740993},"reasoning_content":"step"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	handler := func(c *gin.Context) {
		request, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, input, string(request))
		response := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "Content-Length": []string{strconv.Itoa(len(upstream))}},
			Body:       io.NopCloser(strings.NewReader(upstream)),
		}
		info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{}}
		usage, apiErr := OpenaiHandler(c, info, response)
		require.Nil(t, apiErr)
		assert.Equal(t, 3, usage.TotalTokens)
	}
	engine := gin.New()
	engine.POST("/v1/chat/completions", handler)
	engine.POST("/pg/chat/completions", handler)
	engine.POST("/v1/completions", handler)
	for path, want := range map[string]string{
		"/v1/chat/completions": extended,
		"/pg/chat/completions": upstream,
		"/v1/completions":      upstream,
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(input)))
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, want, recorder.Body.String())
			assert.Equal(t, strconv.Itoa(len(want)), recorder.Header().Get("Content-Length"))
		})
	}
}

func TestChatCompletionReasoningStreamKeepsFramingAndThinkingMode(t *testing.T) {
	const reason = `{"choices":[{"index":0,"delta":{"reasoning":"step"}}]}`
	const answer = `{"choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`
	const usage = `{"choices":[],"usage":{"total_tokens":3}}`
	for _, test := range []struct {
		name           string
		forceFormat    bool
		thinkToContent bool
		wantReasoning  string
		wantContent    []string
	}{
		{name: "passthrough", wantReasoning: "step", wantContent: []string{"answer"}},
		{name: "forced format", forceFormat: true, wantReasoning: "step", wantContent: []string{"answer"}},
		{name: "thinking to content", thinkToContent: true, wantContent: []string{"<think>\nstep", "\n</think>\nanswer"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				info := &relaycommon.RelayInfo{}
				require.NoError(t, sendStreamData(c, info, reason, test.forceFormat, test.thinkToContent))
				require.NoError(t, sendStreamData(c, info, answer, test.forceFormat, test.thinkToContent))
				require.NoError(t, helper.StringData(c, usage))
				helper.Done(c)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			require.True(t, recorder.Flushed)
			frames := strings.Split(strings.TrimSuffix(recorder.Body.String(), "\n\n"), "\n\n")
			require.GreaterOrEqual(t, len(frames), 4)
			assert.Equal(t, "data: "+usage, frames[len(frames)-2])
			assert.Equal(t, "data: [DONE]", frames[len(frames)-1])
			var contents []string
			var reasoning strings.Builder
			for _, frame := range frames[:len(frames)-2] {
				var chunk struct {
					Choices []struct {
						Delta map[string]common.RawMessage
					}
				}
				require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(frame, "data: "), &chunk))
				require.Len(t, chunk.Choices, 1)
				delta := chunk.Choices[0].Delta
				require.Contains(t, delta, "reasoning_content")
				var thought *string
				require.NoError(t, common.Unmarshal(delta["reasoning_content"], &thought))
				if thought != nil {
					reasoning.WriteString(*thought)
				}
				if raw, ok := delta["content"]; ok {
					var text string
					require.NoError(t, common.Unmarshal(raw, &text))
					contents = append(contents, text)
				}
			}
			assert.Equal(t, test.wantContent, contents)
			assert.Equal(t, test.wantReasoning, reasoning.String())
		})
	}
}

func BenchmarkChatCompletionReasoningSSE(b *testing.B) {
	const frame = `{"id":"fixture","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"short"},"finish_reason":null}]}`
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "original_write", path: "/pg/chat/completions"},
		{name: "with_extension", path: "/v1/chat/completions"},
	} {
		b.Run(test.name, func(b *testing.B) {
			engine := gin.New()
			engine.POST(test.path, func(c *gin.Context) {
				var info relaycommon.RelayInfo
				for range 64 {
					if err := sendStreamData(c, &info, frame, false, false); err != nil {
						b.Fatal(err)
					}
				}
				helper.Done(c)
			})
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			b.ReportAllocs()
			for b.Loop() {
				engine.ServeHTTP(httptest.NewRecorder(), request)
			}
			b.ReportMetric(64, "data_frames/op")
		})
	}
}
