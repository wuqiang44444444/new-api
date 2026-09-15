package seedance

import (
	"context"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPluginProviderObservationsPreserveTerminalEvidence(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	for _, tc := range []struct {
		protocol dto.VideoUpstreamProtocol
		body     string
		legacy   func([]byte, string) ([]byte, error)
	}{
		{dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, `{"task_id":"task","status":"completed","result":{"primary_url":"https://user:pass@invalid.example/video","url":"https://result.example/video"},"usage":{"completion_tokens":0,"total_tokens":20}}`, thirdparty.RelayTaskResponse},
		{dto.VideoUpstreamProtocolMoxingModelArkV1, `{"task_id":"task","status":"completed","result":"{\"urls\":[\"http://invalid.example\",\"https://result.example/video\"]}","usage":{"total_tokens":20,"prompt_tokens":3}}`, thirdparty.RelayTaskResponse},
		{dto.VideoUpstreamProtocolMoxingModelArkV1, `{"task_id":"task","status":"failed","error_message":"api_key=fixture-secret invalid duration"}`, thirdparty.RelayTaskResponse},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, `{"id":"task","status":"succeeded","content":{"video_url":"https://result.example/video","last_frame_url":"https://result.example/frame"},"usage":{"completion_tokens":8}}`, thirdparty.FunCloudModelArkTaskResponse},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, `{"id":"task","status":"failed","error":{"code":"BadRequest","message":"api_key=fixture-secret invalid duration"}}`, thirdparty.FunCloudModelArkTaskResponse},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, `{"id":"task","status":"succeeded","content":{"video_url":"http://invalid.example/video"}}`, thirdparty.FunCloudModelArkTaskResponse},
		{dto.VideoUpstreamProtocolSynlinkVideoV1, `{"task":{"id":"task","status":"completed","outputs":["https://result.example/video"],"duration_seconds":5,"created_at":"2026-09-01T08:00:00+08:00","usage":{"total_tokens":20,"prompt_tokens":4}}}`, thirdparty.SynlinkTaskResponse},
		{dto.VideoUpstreamProtocolSynlinkVideoV1, `{"task":{"id":"task","status":"completed","outputs":["https://user:pass@invalid.example/video"]}}`, thirdparty.SynlinkTaskResponse},
	} {
		t.Run(string(tc.protocol)+tc.body, func(t *testing.T) {
			expected, oldErr := tc.legacy([]byte(tc.body), "task")
			output, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{string(tc.protocol), "parseTaskObservation"}, seedanceObservationInput([]byte(tc.body), "task"))
			require.NoError(t, err)
			actual, err := decodeOfficialPluginObservation(output, "task", plugin.Meta.APIVersion, "tokensave_media_task_v1")
			if err == nil {
				actual, err = validatePluginProviderObservation(actual, tc.protocol, "", "")
			}
			if oldErr != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(actual))
			assert.NotContains(t, string(actual), "fixture-secret")
		})
	}
}
