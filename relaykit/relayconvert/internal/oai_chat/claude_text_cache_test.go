package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeChatPreservesExplicitTextCacheControl(t *testing.T) {
	var input dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{
		"model":"claude-sonnet-4-6","max_tokens":32,
		"messages":[
			{"role":"system","content":[{"type":"text","text":"system","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"without a breakpoint"}]},
			{"role":"system","content":"string system"},
			{"role":"user","content":[{"type":"text","text":"user","cache_control":{"type":"ephemeral"}}]},
			{"role":"assistant","content":[{"type":"text","text":"assistant","cache_control":{"type":"ephemeral"}}],"tool_calls":[{"id":"call_test","type":"function","function":{"name":"lookup","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_test","content":[{"type":"text","text":"tool","cache_control":{"type":"ephemeral"}}]},
			{"role":"user","content":[{"type":"text","text":"null breakpoint","cache_control":null}]}
		]
	}`), &input))
	assert.Empty(t, input.Messages[0].ParseContent()[0].CacheControl)
	wire, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, input)
	require.NoError(t, err)
	require.Len(t, wire.Messages, 4)
	system, err := kitutil.Marshal(wire.System)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type":"text","text":"system","cache_control":{"type":"ephemeral","ttl":"1h"}},
		{"type":"text","text":"without a breakpoint"},
		{"type":"text","text":"string system"}
	]`, string(system))
	for index, want := range []string{
		`[{"type":"text","text":"user","cache_control":{"type":"ephemeral"}}]`,
		`[{"type":"text","text":"assistant","cache_control":{"type":"ephemeral"}},{"type":"tool_use","id":"call_test","name":"lookup","input":{}}]`,
		`[{"type":"tool_result","tool_use_id":"call_test","content":[{"type":"text","text":"tool","cache_control":{"type":"ephemeral"}}]}]`,
		`[{"type":"text","text":"null breakpoint"}]`,
	} {
		content, err := kitutil.Marshal(wire.Messages[index].Content)
		require.NoError(t, err)
		assert.JSONEq(t, want, string(content))
	}
}

func TestClaudeChatRejectsUnencodableTextCacheControl(t *testing.T) {
	input := dto.GeneralOpenAIRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "cached", "cache_control": map[string]any{"type": make(chan int)}},
			}},
		},
	}
	_, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, input)
	require.ErrorContains(t, err, "encode Claude text cache_control")
}
