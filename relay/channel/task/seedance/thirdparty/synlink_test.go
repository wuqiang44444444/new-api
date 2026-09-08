package thirdparty

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkCreateRequiresDocumentedTaskEnvelope(t *testing.T) {
	raw, err := SynlinkCreateResponse([]byte(`{"task":{"id":"syn-1","status":"pending","outputs":[]}}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"syn-1"}`, string(raw))
	for _, body := range []string{`{`, `null`, `{"id":"syn-1"}`, `{"task":{"id":"syn-1"}}`, `{"task":{"id":"syn-1","status":"completed"}}`, `{"task":{"id":"syn-1","status":"pending"},"error":{"code":"rejected"}}`} {
		_, err := SynlinkCreateResponse([]byte(body))
		assert.Error(t, err, body)
	}
}

func TestSynlinkCompletedUsageAndOutput(t *testing.T) {
	raw, err := SynlinkTaskResponse([]byte(`{"task":{"id":"syn-1","status":"completed","outputs":["https://cdn.example/video.mp4"],"duration_seconds":5,"created_at":"2026-09-07T12:15:23.552Z","completed_at":1788783405,"usage":{"completion_tokens":48400,"total_tokens":48400}}}`), "syn-1")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(raw, &result))
	assert.Equal(t, "succeeded", result["status"])
	assert.Equal(t, float64(5), result["duration"])
	assert.Equal(t, float64(1788783323), result["created_at"])
	assert.Equal(t, float64(1788783405), result["updated_at"])
	assert.Equal(t, map[string]any{"video_url": "https://cdn.example/video.mp4"}, result["content"])
	assert.Equal(t, map[string]any{"completion_tokens": float64(48400), "total_tokens": float64(48400)}, result["usage"])
	assert.Equal(t, "task.usage.completion_tokens", result["usage_source"])
	for _, usage := range []string{`null`, `{"completion_tokens":-1,"total_tokens":48400}`, `{"completion_tokens":2147483648}`, `{"completion_tokens":1.5}`} {
		raw, err := SynlinkTaskResponse([]byte(`{"task":{"id":"syn-1","status":"completed","outputs":["https://cdn.example/video.mp4"],"usage":`+usage+`}}`), "syn-1")
		require.NoError(t, err)
		result = nil
		require.NoError(t, common.Unmarshal(raw, &result))
		assert.Equal(t, "succeeded", result["status"])
		assert.NotContains(t, result, "usage")
	}
}

func TestSynlinkUntrustedObservationCannotBecomeFailure(t *testing.T) {
	for _, body := range []string{
		`{"task":{"id":"other","status":"pending"}}`,
		`{"task":{"id":"syn-1","status":"failed"}}`,
		`{"task":{"id":"syn-1","status":"processing"}}`,
		`{"task":{"id":"syn-1","status":"completed","outputs":[]}}`,
		`{"task":{"id":"syn-1","status":"completed","outputs":["https://cdn.example/a","https://cdn.example/b"]}}`,
		`{"task":{"id":"syn-1","status":"completed","outputs":["http://cdn.example/a"]}}`,
	} {
		_, err := SynlinkTaskResponse([]byte(body), "syn-1")
		assert.Error(t, err, body)
	}
	raw, err := SynlinkTaskResponse([]byte(`{"task":{"id":"syn-1","status":"pending"}}`), "syn-1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"syn-1","status":"queued"}`, string(raw))
}
