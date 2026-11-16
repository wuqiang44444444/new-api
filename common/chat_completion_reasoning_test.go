package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompleteChatReasoningPreservesResponseValues(t *testing.T) {
	t.Run("multiple choices preserve existing values", func(t *testing.T) {
		input := ` { "choices":[{"index":0,"message":{"content":"answer","reasoning":"\u0020\n","extra":{"number":9007199254740993}}},{"message":{"reasoning_content":null,"reasoning":"ignore"}},{"message":{"reasoning_content":"","reasoning":"ignore"}},{"message":{"reasoning_content":"keep"}},{"message":{}},{"message":{"reasoning":{"text":"unknown"}}},{"text":"legacy"},{"message":null}], "meta": 12345678901234567890123456789 } `
		expected := ` { "choices":[{"index":0,"message":{"content":"answer","reasoning":"\u0020\n","extra":{"number":9007199254740993},"reasoning_content":"\u0020\n"}},{"message":{"reasoning_content":null,"reasoning":"ignore"}},{"message":{"reasoning_content":"","reasoning":"ignore"}},{"message":{"reasoning_content":"keep"}},{"message":{"reasoning_content":null}},{"message":{"reasoning":{"text":"unknown"},"reasoning_content":null}},{"text":"legacy"},{"message":null}], "meta": 12345678901234567890123456789 } `
		patched := completeChatReasoning(input, "message")
		assert.Equal(t, expected, patched)
		assert.Empty(t, completeChatReasoning(patched, "message"))
	})

	t.Run("stream keeps role, tools and usage", func(t *testing.T) {
		input := `{"choices":[{"delta":{"role":"assistant"}},{"delta":{"tool_calls":[{"function":{"arguments":"{\"a\":1}"}}]}}],"usage":{"completion_tokens":1}}`
		expected := `{"choices":[{"delta":{"role":"assistant","reasoning_content":null}},{"delta":{"tool_calls":[{"function":{"arguments":"{\"a\":1}"}}],"reasoning_content":null}}],"usage":{"completion_tokens":1}}`
		assert.Equal(t, expected, completeChatReasoning(input, "delta"))
	})

	for _, input := range []string{
		`[DONE]`,
		`{"choices":[],"usage":{"total_tokens":3}}`,
		`{"error":{"code":"failure"},"choices":[{"delta":{}}]}`,
		`{"choices":[{"delta":{}}]} invalid`,
		`{"output":[{"type":"reasoning"}]}`,
	} {
		assert.Empty(t, completeChatReasoning(input, "delta"), input)
	}
}

func BenchmarkCompleteChatReasoning(b *testing.B) {
	for name, frame := range map[string]string{
		"body":    `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"short"},"finish_reason":null}]}`,
		"alias":   `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning":"step"},"finish_reason":null}]}`,
		"present": `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"short","reasoning_content":null},"finish_reason":null}]}`,
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = completeChatReasoning(frame, "delta")
			}
		})
	}
}
