package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkPluginTaskStatusContract(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	for _, tc := range []struct {
		name, body string
		status     model.TaskStatus
	}{
		{"pending", `{"task":{"id":"task","status":"pending"}}`, model.TaskStatusQueued},
		{"processing without metadata", `{"task":{"id":"task","status":"processing"}}`, model.TaskStatusInProgress},
		{"processing ignores premature completion", `{"task":{"id":"task","status":"processing","outputs":["https://result.example/video"],"usage":{"total_tokens":20},"metadata":{"status":"succeeded","usage":{"total_tokens":20}}}}`, model.TaskStatusInProgress},
		{"completed uses wrapper usage once", `{"task":{"id":"task","status":"completed","outputs":["https://result.example/video"],"usage":{"total_tokens":20},"metadata":{"status":"succeeded","usage":{"total_tokens":20}}}}`, model.TaskStatusSuccess},
		{"unknown", `{"task":{"id":"task","status":"mystery"}}`, ""},
		{"missing", `{"task":{"id":"task"}}`, ""},
		{"wrong type", `{"task":{"id":"task","status":1}}`, ""},
		{"mismatched identity", `{"task":{"id":"other","status":"processing"}}`, ""},
		{"task error", `{"task":{"id":"task","status":"processing","error":{}}}`, ""},
		{"query error", `{"error":{},"task":{"id":"task","status":"processing"}}`, ""},
		{"missing output", `{"task":{"id":"task","status":"completed"}}`, ""},
		{"unsafe output", `{"task":{"id":"task","status":"completed","outputs":["http://result.example/video"]}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{string(dto.VideoUpstreamProtocolSynlinkVideoV1), "parseTaskObservation"}, seedanceObservationInput([]byte(tc.body), "task"))
			require.NoError(t, err)
			normalized, err := decodeOfficialPluginObservation(output, "task", plugin.Meta.APIVersion, dto.VideoUpstreamProtocolSynlinkVideoV1)
			if err == nil {
				normalized, err = validatePluginProviderObservation(normalized, dto.VideoUpstreamProtocolSynlinkVideoV1)
			}
			if tc.status == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			info, err := (&TaskAdaptor{}).ParseTaskResult(nil, nil, normalized)
			require.NoError(t, err)
			assert.Equal(t, string(tc.status), info.Status)
			if tc.status == model.TaskStatusSuccess {
				assert.True(t, info.UsageReported)
				assert.Equal(t, 20, info.TotalTokens)
			} else {
				assert.False(t, info.UsageReported)
				assert.NotContains(t, string(normalized), "video_url")
				assert.NotContains(t, string(normalized), "usage")
			}
		})
	}
}
