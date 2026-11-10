package minimax

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexhinaHailuoV2Observation(t *testing.T) {
	plugin := compileTestArtifact(t)
	const protocol = constant.VideoUpstreamProtocolNexhinaHailuoV2
	createPath, queryPath, registered := constant.MiniMaxVideoPaths(protocol)
	require.True(t, registered)
	assert.Equal(t, "/hailuo/v2/video_generation", createPath)
	assert.Equal(t, "/hailuo/v2/query/video_generation/{task_id}", queryPath)
	task := &model.Task{
		Platform:    Platform,
		PrivateData: model.TaskPrivateData{VideoUpstreamProtocol: protocol},
	}
	task.Properties.UpstreamModelName = "configured-provider-route"
	for _, status := range []string{"processing", "running"} {
		t.Run(status, func(t *testing.T) {
			body := []byte(`{"task":{"id":"task-fixture","model":"provider-display-model","status":"` + status + `"}}`)
			observation, safe, err := normalizeTaskObservation(context.Background(), plugin, task, body, "task-fixture")
			require.NoError(t, err)
			assert.Equal(t, observationStatusRunning, observation.Status)
			assert.NotContains(t, string(safe), "task-fixture")
		})
	}

	for _, body := range []string{
		`{"task":{"id":"different-task","model":"configured-provider-route","status":"running"}}`,
		`{"task":{"id":"task-fixture","task_type":"other","status":"running"}}`,
		`{"task":{"id":"task-fixture","modality":"image","status":"running"}}`,
	} {
		_, _, err := normalizeTaskObservation(context.Background(), plugin, task, []byte(body), "task-fixture")
		require.Error(t, err)
	}

	task.PrivateData.VideoUpstreamProtocol = constant.VideoUpstreamProtocolMiniMaxOfficialV2
	for _, body := range []string{
		`{"task":{"id":"task-fixture","model":"configured-provider-route","status":"processing"}}`,
		`{"task":{"id":"task-fixture","model":"provider-display-model","status":"running"}}`,
	} {
		_, _, err := normalizeTaskObservation(context.Background(), plugin, task, []byte(body), "task-fixture")
		require.Error(t, err)
	}
}

func TestNexhinaHailuoV2ReferenceImageWireContract(t *testing.T) {
	plugin, extension, err := jsplugin.CompileSeedanceExtension(plugins.MinimaxSource(), jsplugin.Options{}, jsplugin.MinimaxHostContract())
	require.NoError(t, err)
	const protocol = constant.VideoUpstreamProtocolNexhinaHailuoV2
	for _, tc := range []struct {
		name, media string
		valid       bool
	}{
		{"reference image", `{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.test/scene.png"}}`, true},
		{"first frame", `{"type":"image_url","role":"first_frame","image_url":{"url":"https://example.test/scene.png"}}`, false},
		{"reference video", `{"type":"video_url","role":"reference_video","video_url":{"url":"https://example.test/scene.mp4"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := `{"model":"pilot","content":[{"type":"text","text":"A landscape"},` + tc.media + `]}`
			var contract taskdto.ModelArkVideoCreateRequest
			require.NoError(t, common.UnmarshalJsonStr(request, &contract))
			var requestMap map[string]any
			require.NoError(t, common.UnmarshalJsonStr(request, &requestMap))
			result, err := plugin.Engine.CallPathWithAdmissionTimeout(context.Background(), createAdmissionTimeout,
				hookRoot, []string{protocol, "buildCreate"}, map[string]any{
					"protocol": protocol, "providerModel": "MiniMax-H3", "request": requestMap,
					"limits": map[string]any{"maxDurationSeconds": 15},
				})
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			conversion, err := decodeCreateConversion(result, extension.Configuration, protocol, "MiniMax-H3", &contract)
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"MiniMax-H3","duration":6,"resolution":"768P","ratio":"16:9","content":[{"type":"text","text":"A landscape"},{"type":"image_url","image_url":{"url":"https://example.test/scene.png"}}]}`, string(conversion.body))
			require.NotNil(t, contract.Content[1].Role)
			assert.Equal(t, "reference_image", *contract.Content[1].Role)
		})
	}
}
