package thirdparty

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	require.NoError(t, common.Unmarshal(body, &value))
	return value
}

func TestCreateResponsesNormalizeTaskID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		transform func([]byte) ([]byte, error)
	}{
		{name: "reverse_proxy", input: `{"data":{"task_id":"reverse_proxy-task-1"}}`, transform: ReverseProxyCreateResponse},
		{name: "relay", input: `{"data":{"task_id":"relay-task-1"}}`, transform: RelayCreateResponse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := test.transform([]byte(test.input))
			require.NoError(t, err)
			assert.Equal(t, test.name+"-task-1", decodeObject(t, body)["id"])
		})
	}
}

func TestReverseProxyTaskResponseNormalizesStatusUsageAndResult(t *testing.T) {
	body, err := ReverseProxyTaskResponse([]byte(`{"data":{"task_id":"rp-1","status":"completed","content":{"video_url":"https://cdn.example/video.mp4"},"usage":{"completion_tokens":123,"total_tokens":456,"prompt_tokens":99}}}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "succeeded", result["status"])
	assert.Equal(t, map[string]any{"video_url": "https://cdn.example/video.mp4"}, result["content"])
	assert.Equal(t, map[string]any{"completion_tokens": float64(123), "total_tokens": float64(456)}, result["usage"])
}

func TestRelayCreateRequestMapsModesAndPreservesExplicitZero(t *testing.T) {
	body, err := RelayCreateRequest([]byte(`{"model":"seedance-v2","content":[{"type":"text","text":"hello"}],"duration":0,"generate_audio":false}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "video_generation", result["capability"])
	assert.Equal(t, "text", result["input_mode"])
	assert.Equal(t, "none", result["control_mode"])
	assert.Equal(t, float64(0), result["duration_seconds"])
	assert.Equal(t, false, result["generate_audio"])
}

func TestRelayCreateRequestMapsFrameControls(t *testing.T) {
	body, err := RelayCreateRequest([]byte(`{"model":"seedance-v2","content":[{"type":"image_url","role":"first_frame","image_url":{"url":"https://cdn.example/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://cdn.example/last.png"}},{"type":"text","text":"move"}]}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "multi_image", result["input_mode"])
	assert.Equal(t, "end_frame", result["control_mode"])
	assert.Equal(t, "https://cdn.example/first.png", result["image"])
	assert.Equal(t, "https://cdn.example/last.png", result["end_image"])
}

func TestRelayCreateRequestRejectsUnsupportedInputs(t *testing.T) {
	tests := []string{
		`{"model":"seedance-v2","content":[{"type":"video_url","video_url":{"url":"https://cdn.example/video.mp4"}}]}`,
		`{"model":"seedance-v2","content":[{"type":"image_url","role":"last_frame","image_url":{"url":"https://cdn.example/last.png"}}]}`,
		`{"model":"seedance-v2","content":[{"type":"image_url","image_url":{"url":"https://cdn.example/one.png"}},{"type":"image_url","image_url":{"url":"https://cdn.example/two.png"}}]}`,
	}
	for _, body := range tests {
		_, err := RelayCreateRequest([]byte(body))
		require.Error(t, err)
	}
}

func TestRelayTaskResponseNormalizesResultWithoutUnverifiedUsage(t *testing.T) {
	body, err := RelayTaskResponse([]byte(`{"task_id":"relay-1","status":"succeeded","result":{"type":"video","urls":["https://cdn.example/result.mp4"]},"usage":{"completion_tokens":999}}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "succeeded", result["status"])
	assert.Equal(t, "https://cdn.example/result.mp4", result["content"].(map[string]any)["video_url"])
	assert.NotContains(t, result, "usage")
}

func TestRelayTaskResponseEnforcesTerminalContracts(t *testing.T) {
	_, err := RelayTaskResponse([]byte(`{"task_id":"relay-1","status":"succeeded","result":{"type":"video"}}`))
	require.Error(t, err)

	failed, err := RelayTaskResponse([]byte(`{"task_id":"relay-2","status":"failed"}`))
	require.NoError(t, err)
	assert.Equal(t, "upstream task failed", decodeObject(t, failed)["error"].(map[string]any)["message"])

	_, err = RelayTaskResponse([]byte(`{"task_id":"relay-3","status":"internal_dispatching"}`))
	require.Error(t, err)
}

// TestReverseProxyTaskResponseEnforcesSucceededContract 验证反代 succeeded 缺结果 URL 时
// fail closed，与中转协议终态合同一致（方案 §3.3）。
func TestReverseProxyTaskResponseEnforcesSucceededContract(t *testing.T) {
	_, err := ReverseProxyTaskResponse([]byte(`{"data":{"task_id":"rp-1","status":"succeeded","content":{}}}`))
	require.Error(t, err)

	body, err := ReverseProxyTaskResponse([]byte(`{"data":{"task_id":"rp-2","status":"succeeded","content":{"video_url":"https://cdn.example/rp.mp4"}}}`))
	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "succeeded", result["status"])
	assert.Equal(t, map[string]any{"video_url": "https://cdn.example/rp.mp4"}, result["content"])
}

// TestReverseProxyTaskResponseRejectsUnknownStatus 验证反代未知状态 fail closed（P1-A）：
// 未知状态必须报错而非原样返回，否则 adaptor ParseTaskResult 会把它当作 IN_PROGRESS 永久轮询、永不结算。
func TestReverseProxyTaskResponseRejectsUnknownStatus(t *testing.T) {
	_, err := ReverseProxyTaskResponse([]byte(`{"data":{"task_id":"rp-x","status":"internal_dispatching"}}`))
	require.Error(t, err)

	_, err = ReverseProxyTaskResponse([]byte(`{"data":{"task_id":"rp-y","status":""}}`))
	require.Error(t, err)
}
