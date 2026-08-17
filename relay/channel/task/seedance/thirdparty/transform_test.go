// Package thirdparty implements code-backed Seedance transport protocols.
package thirdparty

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

func TestRelayCreateRequestUsesMediaTaskFieldNamesAndPreservesExplicitZero(t *testing.T) {
	body, err := RelayCreateRequest([]byte(`{"model":"seedance-v2","content":[{"type":"text","text":"hello"}],"duration":0,"generate_audio":false,"ratio":"16:9"}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "video_generation", result["capability"])
	assert.Equal(t, "text", result["input_mode"])
	assert.Equal(t, "none", result["control_mode"])
	assert.Equal(t, float64(0), result["duration_seconds"])
	assert.Equal(t, false, result["with_audio"])
	assert.Equal(t, "16:9", result["aspect_ratio"])
	assert.NotContains(t, result, "generate_audio")
	assert.NotContains(t, result, "ratio")
}

func TestRelayCreateRequestUsesCurrentDoubaoSeedanceContract(t *testing.T) {
	body, err := RelayCreateRequest([]byte(`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"snow mountain"}],"duration":5,"resolution":"720p","ratio":"16:9","generate_audio":false}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "doubao-seedance-2-0-260128", result["model"])
	assert.Equal(t, "video_generation", result["capability"])
	assert.Equal(t, "text", result["input_mode"])
	assert.Equal(t, "none", result["control_mode"])
	assert.Equal(t, "snow mountain", result["prompt"])
	assert.Equal(t, float64(5), result["duration_seconds"])
	assert.Equal(t, "720p", result["resolution"])
	assert.Equal(t, "16:9", result["aspect_ratio"])
	assert.Equal(t, false, result["with_audio"])
}

func TestRelayCreateRequestDoesNotSilentlyDropOptionalFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "doubao model preserves explicit optional fields",
			body: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"hello"}],"seed":0,"camera_fixed":false,"watermark":false}`,
		},
		{
			name: "oversea model preserves explicit optional fields",
			body: `{"model":"seedance-2-0-oversea","content":[{"type":"text","text":"hello"}],"seed":0,"camera_fixed":false,"watermark":false}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := RelayCreateRequest([]byte(test.body))
			require.NoError(t, err)

			result := decodeObject(t, body)
			assert.Equal(t, float64(0), result["seed"])
			assert.Equal(t, false, result["camera_fixed"])
			assert.Equal(t, false, result["watermark"])
		})
	}
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

func TestMoxingMediaCreateRequestPreservesReferenceAudioAndVideo(t *testing.T) {
	body, err := MoxingMediaCreateRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"follow the references"},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example/reference.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://cdn.example/reference.mp3"}}
		]
	}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "multi_modal", result["input_mode"])
	assert.Equal(t, "reference", result["control_mode"])
	assert.Equal(t, []any{"https://cdn.example/reference.mp4"}, result["reference_videos"])
	assert.Equal(t, []any{"https://cdn.example/reference.mp3"}, result["reference_audios"])
}

func TestRelayTaskResponseAlwaysUsesTerminalUsage(t *testing.T) {
	providerBody := []byte(`{"task_id":"relay-1","status":"succeeded","result":{"type":"video","urls":["https://cdn.example/result.mp4"]},"usage":{"completion_tokens":999,"total_tokens":999}}`)
	body, err := RelayTaskResponse(providerBody, "relay-1")

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "succeeded", result["status"])
	assert.Equal(t, "https://cdn.example/result.mp4", result["content"].(map[string]any)["video_url"])
	assert.Equal(t, map[string]any{"completion_tokens": float64(999), "total_tokens": float64(999)}, result["usage"])
}

func TestRelayTaskResponseAcceptsDocumentedStringResultAndIgnoresNonNumericUsage(t *testing.T) {
	body, err := RelayTaskResponse(
		[]byte(`{"task_id":"relay-1","status":"succeeded","result":"{\"url\":\"https://cdn.example/result.mp4\",\"duration_seconds\":4}","usage":"provider-defined"}`),
		"relay-1",
	)

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "https://cdn.example/result.mp4", result["content"].(map[string]any)["video_url"])
	assert.NotContains(t, result, "usage")
	assert.NotContains(t, result, "usage_evidence")
}

func TestRelayTaskResponseNormalizesMoxingTerminalUsage(t *testing.T) {
	body, err := RelayTaskResponse(
		[]byte(`{"object":"media.task","task_id":"moxing-1","status":"succeeded","status_code":200,"model":"doubao-seedance-2-0-fast-260128","result":{"type":"video","primary_url":"https://cdn.example/result.mp4"},"usage":{"completion_tokens":40594,"completion_tokens_details":{"reasoning_tokens":0},"prompt_tokens":0,"prompt_tokens_details":{"cached_tokens":0},"total_tokens":40594}}`),
		"moxing-1",
	)

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, map[string]any{"completion_tokens": float64(40594), "total_tokens": float64(40594)}, result["usage"])
	assert.Equal(t, "usage.completion_tokens", result["usage_source"])
	evidence := result["usage_evidence"].(map[string]any)
	assert.Equal(t, float64(40594), evidence["usage.completion_tokens"])
	assert.Equal(t, float64(0), evidence["usage.prompt_tokens_details.cached_tokens"])
}

func TestRelayTaskResponseFormsBillableUsageOnlyFromVerifiedFields(t *testing.T) {
	tests := []struct {
		name           string
		usage          string
		wantCompletion float64
		wantTotal      float64
		wantSource     string
	}{
		{name: "root output token", usage: `null,"output_tokens":2345`, wantCompletion: 2345, wantTotal: 2345, wantSource: "output_tokens"},
		{name: "invalid completion falls back to total", usage: `{"completion_tokens":-1,"total_tokens":4567}`, wantCompletion: 4567, wantTotal: 4567, wantSource: "usage.total_tokens"},
		{name: "total minus prompt", usage: `{"prompt_tokens":100,"total_tokens":500}`, wantCompletion: 400, wantTotal: 500, wantSource: "usage.total_tokens-usage.prompt_tokens"},
		{name: "explicit completion zero", usage: `{"completion_tokens":0}`, wantCompletion: 0, wantTotal: 0, wantSource: "usage.completion_tokens"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := RelayTaskResponse(
				[]byte(`{"task_id":"relay-usage","status":"succeeded","result":{"primary_url":"https://cdn.example/result.mp4"},"usage":`+test.usage+`}`),
				"relay-usage",
			)

			require.NoError(t, err)
			result := decodeObject(t, body)
			usage := result["usage"].(map[string]any)
			assert.Equal(t, test.wantCompletion, usage["completion_tokens"])
			assert.Equal(t, test.wantTotal, usage["total_tokens"])
			assert.Equal(t, test.wantSource, result["usage_source"])
		})
	}
}

func TestRelayTaskResponseKeepsUnverifiedUsageAsEvidenceOnly(t *testing.T) {
	tests := []struct {
		name         string
		usage        string
		evidencePath string
		evidenceWant float64
	}{
		{name: "prompt-only usage", usage: `{"prompt_tokens":120}`, evidencePath: "usage.prompt_tokens", evidenceWant: 120},
		{name: "inconsistent total below prompt", usage: `{"prompt_tokens":200,"total_tokens":100}`, evidencePath: "usage.total_tokens", evidenceWant: 100},
		{name: "details-only zero usage", usage: `{"completion_tokens_details":{"reasoning_tokens":0}}`, evidencePath: "usage.completion_tokens_details.reasoning_tokens", evidenceWant: 0},
		{name: "generic numeric usage", usage: `{"consumed":3456}`, evidencePath: "usage.consumed", evidenceWant: 3456},
		{name: "JSON string usage", usage: `"{\"video_token_usage\":1234}"`, evidencePath: "usage.video_token_usage", evidenceWant: 1234},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := RelayTaskResponse(
				[]byte(`{"task_id":"relay-usage","status":"succeeded","result":{"primary_url":"https://cdn.example/result.mp4"},"usage":`+test.usage+`}`),
				"relay-usage",
			)

			require.NoError(t, err)
			result := decodeObject(t, body)
			assert.NotContains(t, result, "usage")
			assert.NotContains(t, result, "usage_source")
			evidence, ok := result["usage_evidence"].(map[string]any)
			require.True(t, ok, "usage evidence must be recorded")
			assert.Equal(t, test.evidenceWant, evidence[test.evidencePath])
		})
	}
}

func TestRelayTaskResponseDoesNotUseNonTerminalUsage(t *testing.T) {
	body, err := RelayTaskResponse(
		[]byte(`{"task_id":"relay-running","status":"running","usage":{"completion_tokens":999}}`),
		"relay-running",
	)

	require.NoError(t, err)
	assert.NotContains(t, decodeObject(t, body), "usage")
}

func TestRelayTaskResponseUsesTerminalUsageOutsideDataEnvelope(t *testing.T) {
	body, err := RelayTaskResponse(
		[]byte(`{"data":{"task_id":"relay-envelope","status":"succeeded","result":{"primary_url":"https://cdn.example/result.mp4"}},"usage":{"completion_tokens":5678}}`),
		"relay-envelope",
	)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"completion_tokens": float64(5678), "total_tokens": float64(5678)}, decodeObject(t, body)["usage"])
}

func TestRelayTaskResponseEnforcesTerminalContracts(t *testing.T) {
	_, err := RelayTaskResponse([]byte(`{"task_id":"relay-1","status":"succeeded","result":{"type":"video"}}`), "relay-1")
	require.Error(t, err)

	failed, err := RelayTaskResponse([]byte(`{"task_id":"relay-2","status":"failed"}`), "relay-2")
	require.NoError(t, err)
	assert.Equal(t, "upstream task failed", decodeObject(t, failed)["error"].(map[string]any)["message"])

	_, err = RelayTaskResponse([]byte(`{"task_id":"relay-3","status":"internal_dispatching"}`), "relay-3")
	require.Error(t, err)
}

func TestRelayResponsesEnforceTaskIdentityAndSafeResultURL(t *testing.T) {
	_, err := RelayCreateResponse([]byte(`{"task_id":"bad\u000aid"}`))
	require.Error(t, err)
	_, err = RelayCreateResponse([]byte(`{"task_id":"` + strings.Repeat("x", 192) + `"}`))
	require.Error(t, err)

	_, err = RelayTaskResponse(
		[]byte(`{"task_id":"different","status":"running"}`),
		"expected",
	)
	require.Error(t, err)
	var violation *relaycommon.UpstreamContractViolation
	assert.ErrorAs(t, err, &violation)

	_, err = RelayTaskResponse(
		[]byte(`{"task_id":"expected","status":"succeeded","result":{"primary_url":"http://cdn.example/result.mp4"}}`),
		"expected",
	)
	require.Error(t, err)
	assert.ErrorAs(t, err, &violation)
}

func TestRelayTaskResponseV1RemainsAvailableOnlyForFrozenLegacyTasks(t *testing.T) {
	body, err := RelayTaskResponseV1([]byte(`{"task_id":"legacy","status":"succeeded","result":{"urls":["https://cdn.example/legacy.mp4"]},"usage":{"total_tokens":42}}`))
	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "legacy", result["id"])
	assert.Equal(t, map[string]any{"total_tokens": float64(42)}, result["usage"])
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

// TestReverseProxyTaskResponseMapsExpiredToFailed 验证官Key（Ark 直通）的 expired 终态被
// 归一化为 failed 触发退款，而非落入 default 报错或被当作 IN_PROGRESS 永久轮询。
// expired 语义：任务过期/超时被清理，无可用结果 URL（方案 §10.6）。
func TestReverseProxyTaskResponseMapsExpiredToFailed(t *testing.T) {
	body, err := ReverseProxyTaskResponse([]byte(`{"data":{"task_id":"rp-exp","status":"expired"}}`))
	require.NoError(t, err)
	assert.Equal(t, "failed", decodeObject(t, body)["status"])
}
