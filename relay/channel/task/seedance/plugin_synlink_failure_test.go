package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkPluginFailedObservation(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	for _, tc := range []struct {
		name, body, message, code, violation string
	}{
		{
			name:    "verified failure envelope",
			body:    `{"task":{"id":"task","status":"failed","error":"The service encountered an unexpected internal error","outputs":[],"metadata":{"id":"internal-task","status":"failed","error":{"code":"InternalServiceError","message":"private internal detail"},"model":"private-model"}}}`,
			message: "The service encountered an unexpected internal error",
		},
		{name: "missing optional error", body: `{"task":{"id":"task","status":"failed"}}`, message: "Video generation failed"},
		{name: "null optional error", body: `{"task":{"id":"task","status":"failed","error":null,"outputs":null}}`, message: "Video generation failed"},
		{name: "blank error", body: `{"task":{"id":"task","status":"failed","error":"  ","outputs":[]}}`, message: "Video generation failed"},
		{name: "metadata cannot replace wrapper failure", body: `{"task":{"id":"task","status":"failed","error":"Generation rejected","metadata":{"status":"succeeded","outputs":["https://result.example/video"],"usage":{"total_tokens":20}},"usage":{"total_tokens":20}}}`, message: "Generation rejected"},
		{name: "identity mismatch", body: `{"task":{"id":"other","status":"failed","error":"Failure"}}`, violation: "Synlink task id mismatch"},
		{name: "query error is not task failure", body: `{"error":"Query unavailable","task":{"id":"task","status":"failed","error":"Failure"}}`, violation: "untrusted Synlink query response"},
		{name: "verified object error without metadata", body: `{"task":{"id":"task","status":"failed","error":{"code":"InternalServiceError","message":"The service encountered an unexpected internal error."},"outputs":[],"created_at":1700000000,"completed_at":1700000060,"model":"private-model"}}`, message: "The service encountered an unexpected internal error.", code: "InternalServiceError"},
		{name: "object message without code", body: `{"task":{"id":"task","status":"failed","error":{"message":"Failure"}}}`, message: "Failure"},
		{name: "object code without message", body: `{"task":{"id":"task","status":"failed","error":{"code":"InternalServiceError"}}}`, message: "Video generation failed", code: "InternalServiceError"},
		{name: "empty object error", body: `{"task":{"id":"task","status":"failed","error":{}}}`, message: "Video generation failed"},
		{name: "null object fields", body: `{"task":{"id":"task","status":"failed","error":{"code":null,"message":null}}}`, message: "Video generation failed"},
		{name: "blank object fields", body: `{"task":{"id":"task","status":"failed","error":{"code":" ","message":" "}}}`, message: "Video generation failed"},
		{name: "object extra fields ignored", body: `{"task":{"id":"task","status":"failed","error":{"code":" Failed ","message":" Failure ","details":"private diagnostics"},"metadata":{"error":{"message":"private fallback"}}}}`, message: "Failure", code: "Failed"},
		{name: "metadata is not an error fallback", body: `{"task":{"id":"task","status":"failed","metadata":{"error":{"message":"private fallback"}}}}`, message: "Video generation failed"},
		{name: "array error", body: `{"task":{"id":"task","status":"failed","error":[]}}`, violation: "invalid Synlink task failure"},
		{name: "boolean error", body: `{"task":{"id":"task","status":"failed","error":false}}`, violation: "invalid Synlink task failure"},
		{name: "numeric error", body: `{"task":{"id":"task","status":"failed","error":0}}`, violation: "invalid Synlink task failure"},
		{name: "wrong code type", body: `{"task":{"id":"task","status":"failed","error":{"code":1,"message":"Failure"}}}`, violation: "invalid Synlink task failure"},
		{name: "wrong message type", body: `{"task":{"id":"task","status":"failed","error":{"code":"Failed","message":{}}}}`, violation: "invalid Synlink task failure"},
		{name: "object failure with conflicting output", body: `{"task":{"id":"task","status":"failed","error":{"code":"Failed","message":"Failure"},"outputs":["https://result.example/video"]}}`, violation: "invalid Synlink task failure"},
		{name: "failure with output conflicts", body: `{"task":{"id":"task","status":"failed","error":"Failure","outputs":["https://result.example/video"]}}`, violation: "invalid Synlink task failure"},
		{name: "malformed failure outputs", body: `{"task":{"id":"task","status":"failed","error":"Failure","outputs":{}}}`, violation: "invalid Synlink task failure"},
		{name: "error does not override unknown status", body: `{"task":{"id":"task","status":"mystery","error":"Failure"}}`, violation: "unexpected Synlink task error"},
		{name: "processing error is not terminal", body: `{"task":{"id":"task","status":"processing","error":"Failure"}}`, violation: "unexpected Synlink task error"},
		{name: "completed error conflicts", body: `{"task":{"id":"task","status":"completed","error":"Failure","outputs":["https://result.example/video"]}}`, violation: "unexpected Synlink task error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{string(dto.VideoUpstreamProtocolSynlinkVideoV1), "parseTaskObservation"}, seedanceObservationInput([]byte(tc.body), "task"))
			require.NoError(t, err)
			body, err := decodeOfficialPluginObservation(output, "task", plugin.Meta.APIVersion, dto.VideoUpstreamProtocolSynlinkVideoV1)
			if tc.violation != "" {
				var violation *relaycommon.UpstreamContractViolation
				require.ErrorAs(t, err, &violation)
				assert.Equal(t, tc.violation, violation.Reason)
				return
			}
			require.NoError(t, err)
			body, err = validatePluginProviderObservation(body, dto.VideoUpstreamProtocolSynlinkVideoV1, "", "")
			require.NoError(t, err)
			info, err := (&TaskAdaptor{}).ParseTaskResult(nil, nil, body)
			require.NoError(t, err)
			assert.Equal(t, string(model.TaskStatusFailure), info.Status)
			assert.Equal(t, tc.message, info.Reason)
			assert.Equal(t, "100%", info.Progress)
			assert.False(t, info.UsageReported)
			assert.Empty(t, info.Url)
			var normalized map[string]any
			require.NoError(t, common.Unmarshal(body, &normalized))
			expectedCode := tc.code
			if expectedCode == "" {
				expectedCode = "upstream_task_failed"
			}
			assert.Equal(t, map[string]any{"code": expectedCode, "message": tc.message}, normalized["error"])
			assert.NotContains(t, normalized, "metadata")
			assert.NotContains(t, normalized, "content")
			assert.NotContains(t, normalized, "usage")
			assert.NotContains(t, string(body), "private")
		})
	}
}
