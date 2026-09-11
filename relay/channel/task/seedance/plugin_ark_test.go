package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkPluginPreservesTaskUsageAndStatus(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	for _, body := range []string{
		`{"id":"task","status":"processing"}`,
		`{"data":{"task_id":"task","state":"completed","output":{"video_url":"https://output.example/video.mp4"},"usage":{"total_tokens":25,"prompt_tokens":5}}}`,
		`{"id":"task","status":"succeeded","content":{"video_url":"https://output.example/video.mp4"},"usage":{"completion_tokens":0,"total_tokens":20}}`,
		`{"id":"task","status":"succeeded","video_url":"https://output.example/video.mp4","usage":{"completion_tokens":-1,"total_tokens":20}}`,
		`{"id":"task","status":"succeeded","video_url":"https://output.example/video.mp4","usage":{"completion_tokens":2147483648,"total_tokens":20}}`,
		`{"id":"task","status":"succeeded","video_url":"https://output.example/video.mp4","usage":"{\"output_tokens\":\"12\"}"}`,
		`{"id":"task","status":"expired","error":{"message":"failed"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			expected, err := thirdparty.ReverseProxyTaskResponse([]byte(body))
			require.NoError(t, err)
			result, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{"ark_media_v1", "parseTaskObservation"}, map[string]any{"taskId": "task", "body": body})
			require.NoError(t, err)
			actual, err := decodeOfficialPluginObservation(result, "task", plugin.Meta.APIVersion, "ark_media_v1")
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(actual))
		})
	}
}
