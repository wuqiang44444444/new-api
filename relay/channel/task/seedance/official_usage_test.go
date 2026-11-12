package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestModelArkDisplayCountersRequireTypedProviderFields(t *testing.T) {
	const profile = dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3
	for _, tc := range []struct {
		name, fields string
		webSearch    *int
	}{
		{"nested_integer", `"tool_usage":{"web_search":2}`, common.GetPointer(2)},
		{"dotted_key", `"tool_usage.web_search":2`, nil},
		{"numeric_string", `"tool_usage":{"web_search":"2"}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"id":"provider-task","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"usage":{"completion_tokens":12,"prompt_tokens":7,` + tc.fields + `}}`)
			normalized, err := thirdparty.FunCloudModelArkTaskResponse(raw, "provider-task")
			require.NoError(t, err)
			normalized = attachModelArkDisplayUsage(profile, raw, normalized, "provider-task")
			result, err := (&TaskAdaptor{}).ParseTaskResult(&model.Task{ClientProtocol: model.TaskClientProtocolModelArkV3}, nil, normalized)
			require.NoError(t, err)
			assert.Equal(t, 12, result.CompletionTokens)
			require.NotNil(t, result.ModelArkUsage)
			assert.Equal(t, common.GetPointer(7), result.ModelArkUsage.PromptTokens)
			assert.Equal(t, tc.webSearch, result.ModelArkUsage.WebSearch)
		})
	}
}

func TestOfficialModelArkDisplayUsesProviderCountersIndependently(t *testing.T) {
	task := &model.Task{
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		PrivateData:    model.TaskPrivateData{VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial},
	}
	for _, tc := range []struct {
		name       string
		usage      string
		prompt     *int
		webSearch  *int
		completion int
		reported   bool
	}{
		{"complete", `{"prompt_tokens":7,"completion_tokens":12,"total_tokens":19,"tool_usage":{"web_search":2}}`, common.GetPointer(7), common.GetPointer(2), 12, true},
		{"prompt_only", `{"prompt_tokens":7,"tool_usage":{"web_search":0}}`, common.GetPointer(7), common.GetPointer(0), 0, false},
		{"invalid", `{"prompt_tokens":-1,"completion_tokens":12,"tool_usage":{"web_search":"2"}}`, nil, nil, 12, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"id":"provider-task","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"usage":` + tc.usage + `,
				"_modelark_display_usage":{"prompt_tokens":-7},
				"_MODELARK_DISPLAY_USAGE":{"prompt_tokens":-7,"web_search":-3,"input_audio_seconds":99}}`)
			normalized, err := normalizeOfficialTaskUsage(raw, "provider-task")
			require.NoError(t, err)
			normalized = attachModelArkDisplayUsage(dto.VideoUpstreamProfileOfficial, raw, normalized, "provider-task")
			assert.NotContains(t, string(normalized), "_MODELARK_DISPLAY_USAGE")
			result, err := (&TaskAdaptor{}).ParseTaskResult(task, nil, normalized)
			require.NoError(t, err)
			assert.Equal(t, model.TaskStatusSuccess, result.Status)
			assert.Equal(t, tc.completion, result.CompletionTokens)
			assert.Equal(t, tc.reported, result.CompletionTokensReported)
			if tc.prompt == nil && tc.webSearch == nil {
				assert.Nil(t, result.ModelArkUsage)
			} else {
				require.NotNil(t, result.ModelArkUsage)
				assert.Equal(t, tc.prompt, result.ModelArkUsage.PromptTokens)
				assert.Equal(t, tc.webSearch, result.ModelArkUsage.WebSearch)
				assert.Nil(t, result.ModelArkUsage.InputAudioSeconds)
			}
		})
	}
}

func TestModelArkDisplayObservationRequiresExactCarrierKey(t *testing.T) {
	body := []byte(`{"status":"succeeded","usage":{"completion_tokens":12},"_MODELARK_DISPLAY_USAGE":{"prompt_tokens":-7}}`)
	task := &model.Task{ClientProtocol: model.TaskClientProtocolModelArkV3}
	result, err := (&TaskAdaptor{}).ParseTaskResult(task, nil, body)
	require.NoError(t, err)
	assert.Nil(t, result.ModelArkUsage)
	assert.Equal(t, 12, result.CompletionTokens)
}

func TestModelArkRelayDisplayCountersAcrossUsageEnvelopes(t *testing.T) {
	const profile = dto.VideoUpstreamProfileThirdPartyRelay
	for _, tc := range []struct {
		name, inner, outer string
		prompt, webSearch  *int
	}{
		{"top_level", `null`, `{"completion_tokens":12,"prompt_tokens":5,"tool_usage":{"web_search":2}}`, common.GetPointer(5), common.GetPointer(2)},
		{"inside_data", `{"completion_tokens":12,"prompt_tokens":5,"tool_usage":{"web_search":2}}`, `null`, common.GetPointer(5), common.GetPointer(2)},
		{"same_in_both", `{"completion_tokens":12,"prompt_tokens":5,"tool_usage":{"web_search":0}}`, `{"prompt_tokens":5,"tool_usage":{"web_search":0}}`, common.GetPointer(5), common.GetPointer(0)},
		{"conflicting_prompt", `{"completion_tokens":12,"prompt_tokens":5}`, `{"prompt_tokens":7,"tool_usage":{"web_search":2}}`, nil, common.GetPointer(2)},
		{"conflicting_search", `{"completion_tokens":12,"prompt_tokens":5,"tool_usage":{"web_search":0}}`, `{"tool_usage":{"web_search":2}}`, common.GetPointer(5), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"data":{"task_id":"provider-task","status":"succeeded","result":{"primary_url":"https://example.com/video.mp4"},"usage":` + tc.inner + `},"usage":` + tc.outer + `}`)
			normalized, err := thirdparty.RelayTaskResponse(raw, "provider-task")
			require.NoError(t, err)
			normalized = attachModelArkDisplayUsage(profile, raw, normalized, "provider-task")
			task := &model.Task{ClientProtocol: model.TaskClientProtocolModelArkV3}
			result, err := (&TaskAdaptor{}).ParseTaskResult(task, nil, normalized)
			require.NoError(t, err)
			assert.Equal(t, 12, result.CompletionTokens)
			require.NotNil(t, result.ModelArkUsage)
			assert.Equal(t, tc.prompt, result.ModelArkUsage.PromptTokens)
			assert.Equal(t, tc.webSearch, result.ModelArkUsage.WebSearch)
		})
	}
}
