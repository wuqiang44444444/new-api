package minimax

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The host reads the located subtree itself and validates the credit value:
// non-integer, negative and unbounded values form no evidence and never fail
// the observation.
func TestNormalizeCreditUsageBounds(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		scanRoot string
		want     map[string]int
	}{
		{"top level", `{"usage":{"video_output":6}}`, "usage.video_output", map[string]int{"video_output": 6}},
		{"nested", `{"content":[{"usage":{"video_output":7}}]}`, "content.usage.video_output", map[string]int{"video_output": 7}},
		{"fraction", `{"usage":{"video_output":6.5}}`, "usage.video_output", nil},
		{"negative", `{"usage":{"video_output":-1}}`, "usage.video_output", nil},
		{"unbounded", `{"usage":{"video_output":99999999999}}`, "usage.video_output", nil},
		{"missing", `{"usage":{}}`, "usage.video_output", nil},
		{"unknown locator", `{"usage":{"video_output":6}}`, "usage.tokens", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence, source := NormalizeCreditUsage([]byte(tc.body), tc.scanRoot)
			if tc.want == nil {
				assert.Nil(t, evidence)
				assert.Empty(t, source)
				return
			}
			assert.Equal(t, tc.want, evidence)
			assert.Equal(t, CreditUsageSource, source)
		})
	}
}

func observationTask(t *testing.T, version string) *model.Task {
	t.Helper()
	return &model.Task{
		TaskID:   "platform-1",
		Status:   model.TaskStatusInProgress,
		Platform: Platform,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:            "task-1",
			VideoUpstreamQueryBaseURL: "https://modelservice.jdcloud.com",
			Key:                       "frozen-key",
			Execution: &model.TaskExecutionSnapshot{
				TaskPlugin: &model.TaskPluginSnapshot{Key: pluginruntime.MinimaxPluginKey, Version: version},
			},
		},
	}
}

// The stored observation carries status and sanitized failure detail only:
// video URLs and credit values stay out of persistence.
func TestDecodeTaskObservationStoredShape(t *testing.T) {
	plugin := compileTestArtifact(t)
	raw := []byte(`{"task_id":"task-1","task_status":"success","error":{"code":0,"type":"","message":""},"content":[{"video_url":{"url":"https://cdn.example.com/v.mp4?sig=secret"},"usage":{"video_output":6}}]}`)
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		context.Background(), pollAdmissionTimeout,
		hookRoot, []string{protocolName(), "parseTaskObservation"},
		map[string]any{"taskId": "task-1", "body": string(raw)},
	)
	require.NoError(t, callErr)
	observation, stored, err := decodeTaskObservation(result, raw, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example.com/v.mp4?sig=secret", observation.VideoURL)
	assert.Equal(t, map[string]int{"video_output": 6}, observation.CreditEvidence)
	var storedBody map[string]any
	require.NoError(t, common.Unmarshal(stored, &storedBody))
	assert.Equal(t, "succeeded", storedBody["status"])
	assert.NotContains(t, storedBody, "videoUrl", "signed URLs never persist into the task row")
	// The usage evidence travels in the observation body so ParseTaskResult
	// can pick it up; the shared link-video redaction strips these fields
	// before the body reaches task persistence.
	assert.Equal(t, "credit:video_output", storedBody["usage_source"])
	assert.Equal(t, float64(6), storedBody["usage_evidence"].(map[string]any)["video_output"])

	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult(nil, nil, stored)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.Equal(t, "credit:video_output", info.UsageSource)
	assert.Equal(t, map[string]int{"video_output": 6}, info.UsageEvidence)
	assert.Equal(t, 0, info.CompletionTokens)
	assert.False(t, info.UsageReported)
}

// A frozen snapshot from another extension never resolves; the observation
// fails closed as a contract violation.
func TestNormalizeObservationRejectsForeignSnapshot(t *testing.T) {
	task := observationTask(t, "1.0.0")
	task.PrivateData.Execution.TaskPlugin.Key = "seedance-link"
	_, _, err := normalizeTaskObservation(context.Background(), task, []byte(`{}`), "task-1")
	require.Error(t, err)
	var violation *relaycommon.UpstreamContractViolation
	assert.ErrorAs(t, err, &violation)
}

// ParseTaskResult stays monotonic for accepted success facts: one malformed
// later poll can never flip an accepted success into failure or refund.
func TestParseTaskResultMonotonicSuccess(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{Status: model.TaskStatusSuccess}
	_, err := adaptor.ParseTaskResult(task, nil, []byte(`{"id":"task-1","status":"failed","error":{"code":"1","message":"x"}}`))
	require.Error(t, err)
	var violation *relaycommon.UpstreamContractViolation
	assert.ErrorAs(t, err, &violation)

	result, err := adaptor.ParseTaskResult(task, nil, []byte(`{"id":"task-1","status":"succeeded"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
}

// Lifecycle bits stay closed for the first phase; content stays supported.
func TestLifecycleCapabilitiesClosed(t *testing.T) {
	capabilities := (&TaskAdaptor{}).TaskLifecycleCapabilities(nil)
	assert.True(t, capabilities.SupportsContent)
	assert.False(t, capabilities.SupportsCancelQueued)
	assert.False(t, capabilities.SupportsDeleteTerminal)
}
