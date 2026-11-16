package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestThinkingTransitionsPreserveOriginalFrameMetadata(t *testing.T) {
	const firstThought = `{"choices":[{"index":0,"delta":{"reasoning_content":"T1"}}]}`
	const firstAnswer = `{"choices":[{"index":0,"delta":{"content":"A1"}}]}`
	const firstMixed = `{"choices":[{"index":0,"delta":{"reasoning_content":"T1","content":"A1"}}]}`
	const nextThought = `{"choices":[{"index":0,"delta":{"reasoning_content":"思考"}}]}`
	const continuedThought = `{"choices":[{"index":0,"delta":{"reasoning_content":"T3"}}]}`
	const toolCall = `{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}`
	const finalMetadata = `"tool_calls":[` + toolCall + `],"annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":2,"url":"https://example.com"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":3}}`
	const finalAnswer = `{"choices":[{"index":0,"delta":{"content":"答复",` + finalMetadata
	const finalMixed = `{"choices":[{"index":0,"delta":{"reasoning_content":"思考","content":"答复",` + finalMetadata
	const stop = `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`
	const toolOnly = `{"choices":[{"index":0,"delta":{"tool_calls":[` + toolCall + `]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":3}}`

	for _, test := range []struct {
		name         string
		upstream     []string
		wantContent  string
		wantCitation bool
		wantTools    int
	}{
		{
			name:         "mixed then consecutive thought frames",
			upstream:     []string{firstMixed, nextThought, continuedThought, finalAnswer},
			wantContent:  "<think>\nT1\n</think>\nA1<think>\n思考T3\n</think>\n答复",
			wantCitation: true,
			wantTools:    1,
		},
		{
			name:         "answer then new thought segment",
			upstream:     []string{firstThought, firstAnswer, nextThought, finalAnswer},
			wantContent:  "<think>\nT1\n</think>\nA1<think>\n思考\n</think>\n答复",
			wantCitation: true,
			wantTools:    1,
		},
		{
			name:         "answer then another mixed frame",
			upstream:     []string{firstThought, firstAnswer, finalMixed},
			wantContent:  "<think>\nT1\n</think>\nA1<think>\n思考\n</think>\n答复",
			wantCitation: true,
			wantTools:    1,
		},
		{
			name:        "tool-only terminal frame emits no closing tag",
			upstream:    []string{firstThought, toolOnly},
			wantContent: "<think>\nT1",
			wantTools:   1,
		},
		{
			name:        "terminal event while thinking is open",
			upstream:    []string{firstThought, firstAnswer, nextThought, stop},
			wantContent: "<think>\nT1\n</think>\nA1<think>\n思考",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				info := &relaycommon.RelayInfo{}
				for _, frame := range test.upstream {
					require.NoError(t, sendStreamData(c, info, frame, false, true))
				}
				helper.Done(c)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			require.Equal(t, http.StatusOK, recorder.Code)

			var content strings.Builder
			var frames, citations, usages, stops, tools int
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				frame, ok := strings.CutPrefix(line, "data: ")
				if !ok || frame == "[DONE]" {
					continue
				}
				frames++
				delta := gjson.Get(frame, "choices.0.delta")
				assert.Equal(t, "null", delta.Get("reasoning_content").Raw)
				text := delta.Get("content").Str
				content.WriteString(text)
				if usage := gjson.Get(frame, "usage.total_tokens"); usage.Exists() {
					usages++
					assert.EqualValues(t, 3, usage.Int())
				}
				if finish := gjson.Get(frame, "choices.0.finish_reason").Str; finish != "" {
					stops++
					if test.wantTools != 0 {
						assert.Equal(t, "tool_calls", finish)
					} else {
						assert.Equal(t, "stop", finish)
					}
				}
				for _, call := range delta.Get("tool_calls").Array() {
					tools++
					assert.Equal(t, "call_1", call.Get("id").Str)
				}
				if citation := delta.Get("annotations.0.url_citation"); citation.Exists() {
					citations++
					start, end := citation.Get("start_index").Int(), citation.Get("end_index").Int()
					runes := []rune(text)
					require.GreaterOrEqual(t, start, int64(0))
					require.Greater(t, end, start)
					require.LessOrEqual(t, end, int64(len(runes)))
					assert.Equal(t, "答复", string(runes[start:end]))
				}
			}
			assert.Equal(t, test.wantContent, content.String())
			assert.Equal(t, len(test.upstream), frames)
			assert.Equal(t, 1, usages)
			assert.Equal(t, 1, stops)
			assert.Equal(t, test.wantTools, tools)
			if test.wantCitation {
				assert.Equal(t, 1, citations)
			} else {
				assert.Zero(t, citations)
			}
			assert.Contains(t, recorder.Body.String(), "data: [DONE]\n\n")
		})
	}
}

func TestThinkingTransitionsKeepCandidatesIndependent(t *testing.T) {
	engine := gin.New()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{}
		for _, frame := range []string{
			`{"choices":[{"index":0,"delta":{"reasoning_content":"T0"}},{"index":1,"delta":{"content":"A1"}}]}`,
			`{"choices":[{"index":1,"delta":{"reasoning_content":"T1","content":"B1"}},{"index":0,"delta":{"reasoning_content":"U0"}}]}`,
			`{"choices":[{"index":0,"delta":{"content":"A0"}},{"index":1,"delta":{"content":"C1"}}]}`,
		} {
			require.NoError(t, sendStreamData(c, info, frame, false, true))
		}
		helper.Done(c)
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var output []string
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if line == "data: [DONE]" {
			break
		}
		frame, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var contents []string
		for _, choice := range gjson.Get(frame, "choices").Array() {
			contents = append(contents, choice.Get("delta.content").Str)
			assert.Equal(t, "null", choice.Get("delta.reasoning_content").Raw)
		}
		output = append(output, contents...)
	}
	assert.Equal(t, []string{
		"<think>\nT0", "A1",
		"<think>\nT1\n</think>\nB1", "U0",
		"\n</think>\nA0", "C1",
	}, output)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]\n\n")
}
