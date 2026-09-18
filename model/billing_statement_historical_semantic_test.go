package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHistoricalWriterSemanticRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, path, billing string
		chain               []string
		extra               map[string]any
		want                int64
		unknown             bool
	}{
		{"responses", "/v1/responses", "upstream", []string{"OpenAI Responses"}, map[string]any{"cache_creation_tokens": 100}, 1000, false},
		{"messages to chat", "/v1/messages", "upstream", []string{"Claude Messages", "OpenAI Compatible"}, nil, 1000, false},
		{"three stage gemini", "/v1/messages", "billing-usage-gemini", []string{"Claude Messages", "OpenAI Compatible", "Google Gemini"}, nil, 1000, false},
		{"final anthropic", "/v1/chat/completions", "upstream", []string{"OpenAI Compatible", "Claude Messages"}, map[string]any{"claude": true, "cache_tokens": 2000, "cache_creation_tokens": 100}, 3100, false},
		{"ambiguous split", "/v1/responses", "upstream", []string{"OpenAI Responses"}, map[string]any{"cache_creation_tokens_5m": 100}, 0, true},
		{"unknown middle", "/v1/messages", "upstream", []string{"Claude Messages", "Future Protocol", "OpenAI Compatible"}, nil, 0, true},
		{"path mismatch", "/v1/messages", "upstream", []string{"OpenAI Responses"}, nil, 0, true},
		{"local estimate", "/v1/responses", "local", []string{"OpenAI Responses"}, nil, 0, true},
		{"inconsistent cache", "/v1/responses", "upstream", []string{"OpenAI Responses"}, map[string]any{"cache_creation_tokens": 900}, 0, true},
		{"malformed cache", "/v1/responses", "upstream", []string{"OpenAI Responses"}, map[string]any{"cache_creation_tokens": "bad"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := map[string]any{"request_path": tc.path, "request_conversion": tc.chain, "admin_info": map[string]any{"usage_billing_path": tc.billing}, "cache_tokens": 300}
			for k, v := range tc.extra {
				other[k] = v
			}
			raw, err := common.Marshal(other)
			require.NoError(t, err)
			got := CustomerBillingLogRow(&Log{Type: LogTypeConsume, PromptTokens: 1000, Quota: 77, Other: string(raw)})
			assert.Equal(t, tc.want, got.InputTokens)
			assert.Equal(t, tc.unknown, got.InputTokensUnavailable)
			assert.EqualValues(t, 77, got.Quota)
		})
	}
}
