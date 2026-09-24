package minimax

import (
	"context"
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileTestArtifact(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	plugin, _, err := pluginruntime.CompileSeedanceExtension(
		plugins.MinimaxSource(), pluginruntime.Options{}, pluginruntime.MinimaxHostContract(),
	)
	require.NoError(t, err)
	return plugin
}

func callBuildCreate(t *testing.T, plugin *pluginruntime.LoadedPlugin, request map[string]any, providerModel string) (any, error) {
	t.Helper()
	if providerModel == "" {
		providerModel = "MiniMax-H3"
	}
	return plugin.Engine.CallPathWithAdmissionTimeout(
		context.Background(), createAdmissionTimeout,
		hookRoot, []string{protocolName(), "buildCreate"},
		map[string]any{
			"protocol":      protocolName(),
			"providerModel": providerModel,
			"request":       request,
			"limits":        map[string]any{"maxDurationSeconds": 3600},
		},
	)
}

func textRequest(text string) map[string]any {
	return map[string]any{"model": "customer-model", "content": []any{map[string]any{"type": "text", "text": text}}}
}

// The verified wire contract: parameters wrapping, the resolution spelling
// conversion, the ratio default and the fixed southbound policy.
func TestBuildCreateProducesVerifiedJDWireShape(t *testing.T) {
	plugin := compileTestArtifact(t)
	result, err := callBuildCreate(t, plugin, textRequest("a cup in morning light"), "")
	require.NoError(t, err)
	body, ok := result.(map[string]any)["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "MiniMax-H3", body["model"])
	content := body["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "a cup in morning light", content[0].(map[string]any)["text"])
	parameters := body["parameters"].(map[string]any)
	assert.Equal(t, int64(6), parameters["duration"])
	assert.Equal(t, "768P", parameters["resolution"])
	assert.Equal(t, "16:9", parameters["ratio"])
	assert.Equal(t, true, parameters["prompt_optimizer"])
	assert.Equal(t, false, parameters["watermark"])
}

// Explicit values pass through; the resolution keeps its spelling conversion.
func TestBuildCreateKeepsExplicitValues(t *testing.T) {
	plugin := compileTestArtifact(t)
	request := textRequest("prompt")
	request["duration"] = 6
	request["resolution"] = "768p"
	request["ratio"] = "16:9"
	request["watermark"] = false
	result, err := callBuildCreate(t, plugin, request, "")
	require.NoError(t, err)
	parameters := result.(map[string]any)["body"].(map[string]any)["parameters"].(map[string]any)
	assert.Equal(t, int64(6), parameters["duration"])
	assert.Equal(t, "768P", parameters["resolution"])
	assert.Equal(t, "16:9", parameters["ratio"])
	assert.Equal(t, false, parameters["watermark"])
}

// Out-of-open-set values fail closed with the declared-scope message and are
// never silently converted: adaptive, other ratios, other resolutions,
// watermark true, unsupported northbound fields and media nodes.
func TestBuildCreateRejectsUndeclaredScope(t *testing.T) {
	plugin := compileTestArtifact(t)
	cases := []struct {
		name   string
		patch  func(map[string]any)
		expect string
	}{
		{"adaptive ratio", func(r map[string]any) { r["ratio"] = "adaptive" }, "ratio"},
		{"other ratio", func(r map[string]any) { r["ratio"] = "2:1" }, "ratio"},
		{"other resolution", func(r map[string]any) { r["resolution"] = "720p" }, "resolution"},
		{"watermark true", func(r map[string]any) { r["watermark"] = true }, "watermark"},
		{"generate_audio", func(r map[string]any) { r["generate_audio"] = false }, "unsupported"},
		{"seed", func(r map[string]any) { r["seed"] = 1 }, "unsupported"},
		{"media node", func(r map[string]any) {
			r["content"] = []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}}}
		}, "reference_image"},
		{"empty text", func(r map[string]any) {
			r["content"] = []any{map[string]any{"type": "text", "text": "  "}}
		}, "text node"},
		{"two nodes", func(r map[string]any) {
			r["content"] = []any{
				map[string]any{"type": "text", "text": "a"},
				map[string]any{"type": "text", "text": "b"},
			}
		}, "exactly one"},
		{"duration 16", func(r map[string]any) { r["duration"] = 16 }, "duration"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := textRequest("prompt")
			tc.patch(request)
			_, err := callBuildCreate(t, plugin, request, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expect)
		})
	}
}

func callParseCreateResponse(t *testing.T, plugin *pluginruntime.LoadedPlugin, body string) (any, error) {
	t.Helper()
	return plugin.Engine.CallPathWithAdmissionTimeout(
		context.Background(), createAdmissionTimeout,
		hookRoot, []string{protocolName(), "parseCreateResponse"},
		map[string]any{"body": body},
	)
}

// Only the registered acceptance state with a trusted id admits a task.
func TestParseCreateResponseRegisteredAcceptanceOnly(t *testing.T) {
	plugin := compileTestArtifact(t)
	result, err := callParseCreateResponse(t, plugin,
		`{"error":null,"requestId":"req","result":{"message":"ok","status":"pending","task_id":"task-1"}}`)
	require.NoError(t, err)
	object := result.(map[string]any)
	assert.Equal(t, "task-1", object["id"])
	assert.Equal(t, "pending", object["status"])

	rejections := []string{
		`{"error":null,"result":{"status":"running","task_id":"task-1"}}`,
		`{"error":null,"result":{"status":"success","task_id":"task-1"}}`,
		`{"error":{"code":400,"message":"bad"},"result":null}`,
		`{"error":null,"result":{"status":"pending"}}`,
		`{"error":null,"result":null}`,
		`not-json`,
	}
	for _, body := range rejections {
		_, err := callParseCreateResponse(t, plugin, body)
		assert.Error(t, err, "body %s must not be accepted", body)
	}
}

func callParseObservation(t *testing.T, plugin *pluginruntime.LoadedPlugin, body string) (any, error) {
	t.Helper()
	return plugin.Engine.CallPathWithAdmissionTimeout(
		context.Background(), pollAdmissionTimeout,
		hookRoot, []string{protocolName(), "parseTaskObservation"},
		map[string]any{"taskId": "task-1", "body": body},
	)
}

// Query status mapping follows the verified contract; the zero-code empty
// error structure of a real success observation is not a failure.
func TestParseObservationStatusMapping(t *testing.T) {
	plugin := compileTestArtifact(t)
	cases := []struct {
		name       string
		body       string
		wantStatus string
	}{
		{"pending", `{"task_id":"task-1","task_status":"pending"}`, "queued"},
		{"running", `{"task_id":"task-1","task_status":"running"}`, "running"},
		{"cancelled", `{"task_id":"task-1","task_status":"cancelled"}`, "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := callParseObservation(t, plugin, tc.body)
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, result.(map[string]any)["status"])
		})
	}
}

// A verified success carries the video URL and the usage locator; the
// zero-code empty error shape stays a success.
func TestParseObservationSuccessContract(t *testing.T) {
	plugin := compileTestArtifact(t)
	result, err := callParseObservation(t, plugin, `{"task_id":"task-1","task_status":"success","error":{"code":0,"type":"","message":""},"content":[{"id":"a","video_url":{"url":"https://cdn.example.com/v.mp4"},"usage":{"video_output":6}}]}`)
	require.NoError(t, err)
	object := result.(map[string]any)
	assert.Equal(t, "succeeded", object["status"])
	assert.Equal(t, "https://cdn.example.com/v.mp4", object["videoUrl"])
	assert.Equal(t, "content.usage.video_output", object["usageScanRoot"])

	// The documented top-level usage shape locates the same field.
	result, err = callParseObservation(t, plugin, `{"task_id":"task-1","task_status":"success","content":[{"video_url":{"url":"https://cdn.example.com/v.mp4"}}],"usage":{"video_output":6}}`)
	require.NoError(t, err)
	assert.Equal(t, "usage.video_output", result.(map[string]any)["usageScanRoot"])

	// The same success without a usage subtree simply carries no locator.
	result, err = callParseObservation(t, plugin, `{"task_id":"task-1","task_status":"success","content":[{"video_url":{"url":"https://cdn.example.com/v.mp4"}}]}`)
	require.NoError(t, err)
	_, hasScanRoot := result.(map[string]any)["usageScanRoot"]
	assert.False(t, hasScanRoot)
}

// Success observations that cannot be trusted stay untrusted: missing or
// multi-result shapes, non-video results, non-zero conflicting errors and
// identity mismatches never advance the task.
func TestParseObservationUntrustedContracts(t *testing.T) {
	plugin := compileTestArtifact(t)
	bodies := []string{
		`{"task_id":"task-1","task_status":"success","content":[]}`,
		`{"task_id":"task-1","task_status":"success","content":[{"video_url":{"url":"https://cdn.example.com/v.mp4"}},{"video_url":{"url":"https://cdn.example.com/w.mp4"}}]}`,
		`{"task_id":"task-1","task_status":"success","content":[{"cover_url":{"url":"https://cdn.example.com/c.jpg"}}]}`,
		`{"task_id":"task-1","task_status":"success","error":{"code":500,"type":"x","message":"boom"},"content":[{"video_url":{"url":"https://cdn.example.com/v.mp4"}}]}`,
		`{"task_id":"task-2","task_status":"success","content":[{"video_url":{"url":"https://cdn.example.com/v.mp4"}}]}`,
		`{"task_id":"task-1","task_status":"success"}`,
		`{"task_id":"task-1","task_status":"waiting"}`,
		`{"task_id":"task-1","task_status":"failed","error":{"code":"oops","message":"x"}}`,
		`{"task_id":"task-1","task_status":"failed"}`,
	}
	for _, body := range bodies {
		_, err := callParseObservation(t, plugin, body)
		assert.Error(t, err, "body %s must stay untrusted", body)
	}
}

// A trusted failure keeps its sanitized detail.
func TestParseObservationTrustedFailure(t *testing.T) {
	plugin := compileTestArtifact(t)
	result, err := callParseObservation(t, plugin, `{"task_id":"task-1","task_status":"failed","error":{"code":1001,"type":"generation","message":"content policy"}}`)
	require.NoError(t, err)
	object := result.(map[string]any)
	assert.Equal(t, "failed", object["status"])
	detail := object["error"].(map[string]any)
	assert.Equal(t, "1001", detail["code"])
	assert.Equal(t, "content policy", detail["message"])
}
