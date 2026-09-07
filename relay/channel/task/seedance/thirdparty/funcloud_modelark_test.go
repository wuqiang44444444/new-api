package thirdparty

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFunCloudModelArkResultUsage(t *testing.T) {
	cases := []struct {
		name, usage string
		tokens      any
		source      string
	}{
		{"completion", `{"completion_tokens":80,"total_tokens":100,"prompt_tokens":20}`, float64(80), "usage.completion_tokens"},
		{"output", `{"output_tokens":80,"total_tokens":100}`, float64(80), "usage.output_tokens"},
		{"total", `{"total_tokens":100}`, float64(100), "usage.total_tokens"},
		{"difference", `{"total_tokens":100,"prompt_tokens":20}`, float64(80), "usage.total_tokens-usage.prompt_tokens"},
		{"prompt", `{"prompt_tokens":20}`, nil, ""},
		{"inconsistent", `{"total_tokens":10,"prompt_tokens":20}`, nil, ""},
		{"missing", `{}`, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"id":"task-1","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"usage":` + tc.usage + `}`)
			raw, err := FunCloudModelArkTaskResponse(body, "task-1")
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, common.Unmarshal(raw, &result))
			assert.Equal(t, "succeeded", result["status"])
			if tc.tokens == nil {
				assert.Nil(t, result["usage"])
			} else {
				assert.Equal(t, tc.tokens, result["usage"].(map[string]any)["completion_tokens"])
				assert.Equal(t, tc.source, result["usage_source"])
			}
			if tc.name != "missing" {
				assert.NotEmpty(t, result["usage_evidence"])
			}
		})
	}
}

func TestFunCloudModelArkPublicResultPreservesReportedFields(t *testing.T) {
	raw, err := FunCloudModelArkTaskResponse([]byte(`{"id":"task-1","status":"succeeded","seed":123,"generate_audio":false,"frames":121,"framespersecond":24,"content":{"video_url":"https://example.com/video.mp4"}}`), "task-1")
	require.NoError(t, err)
	task := &model.Task{TaskID: "public-id", Status: model.TaskStatusSuccess}
	task.Data = raw
	public := task.ToModelArkVideoTask()
	assert.EqualValues(t, 123, public.Seed)
	require.NotNil(t, public.GenerateAudio)
	assert.False(t, *public.GenerateAudio)
	assert.Equal(t, 121, public.Frames)
	assert.Equal(t, 24, public.FramesPerSecond)
}

func TestFunCloudModelArkRejectsUntrustedResults(t *testing.T) {
	for _, body := range []string{`{`, `{"id":"other","status":"running"}`, `{"id":"task-1","status":"invented"}`, `{"id":"task-1","status":"succeeded"}`, `{"id":"task-1","status":"succeeded","content":{"video_url":"http://127.0.0.1/a"}}`} {
		_, err := FunCloudModelArkTaskResponse([]byte(body), "task-1")
		assert.Error(t, err)
	}
	for _, body := range []string{`{`, `{}`, `{"data":{"id":"task-1"}}`, `{"id":1}`, `{"id":"task-1","error":{"code":"bad"}}`} {
		_, err := FunCloudModelArkCreateResponse([]byte(body))
		assert.Error(t, err)
	}
	raw, err := FunCloudModelArkCreateResponse([]byte(`{"id":"task-1"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task-1"}`, string(raw))
	raw, err = FunCloudModelArkTaskResponse([]byte(`{"id":"task-1","status":"submitted"}`), "task-1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task-1","status":"queued"}`, string(raw))
}
