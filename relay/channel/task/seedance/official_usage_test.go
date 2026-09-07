package seedance

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOfficialUsageRequiresValidCompletionTokens(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		reported    bool
		tokens      int
	}{
		{"missing", `null`, false, 0},
		{"empty", `{}`, false, 0},
		{"total-only", `{"total_tokens":120}`, false, 0},
		{"prompt-total", `{"prompt_tokens":20,"total_tokens":120}`, false, 0},
		{"zero", `{"completion_tokens":0,"total_tokens":20}`, true, 0},
		{"valid", `{"completion_tokens":100,"total_tokens":120}`, true, 100},
		{"negative", `{"completion_tokens":-1,"total_tokens":120}`, false, 0},
		{"overflow", `{"completion_tokens":18446744073709551615}`, false, 0},
		{"fractional", `{"completion_tokens":1.5}`, false, 0},
		{"string", `{"completion_tokens":"100"}`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"id":"provider-task","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"usage":` + tc.usage + `}`)
			normalized, err := normalizeOfficialTaskUsage(body, "provider-task")
			require.NoError(t, err)
			result, err := (&TaskAdaptor{}).ParseTaskResult(nil, nil, normalized)
			require.NoError(t, err)
			assert.EqualValues(t, model.TaskStatusSuccess, result.Status)
			assert.Equal(t, tc.reported, result.UsageReported)
			assert.Equal(t, tc.tokens, result.CompletionTokens)
			if tc.reported {
				assert.Equal(t, "usage.completion_tokens", result.UsageSource)
			}
			var payload map[string]any
			require.NoError(t, common.Unmarshal(normalized, &payload))
			if !tc.reported {
				assert.NotContains(t, payload, "usage")
			}
		})
	}
	_, err := normalizeOfficialTaskUsage([]byte(`{"id":"other-task","status":"succeeded","usage":{"completion_tokens":100}}`), "provider-task")
	require.Error(t, err)
}
