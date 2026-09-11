package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

// TokenSave bills by input_mode/control_mode. The hold-time conversion check
// must reject any converted body whose modes drift from the host billing
// classification of the same northbound content.
func TestTokenSaveCreateRejectsBillingModeDrift(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)

	const providerModel = "doubao-seedance-2-0-260128"
	cases := []struct {
		name          string
		content       string
		expectedMode  [2]string
		driftingField int
	}{
		{name: "text", content: `[{"type":"text","text":"a boat"}]`, expectedMode: [2]string{"text", "none"}},
		{name: "first frame", content: `[{"type":"image_url","role":"first_frame","image_url":{"url":"https://example.com/f.png"}}]`, expectedMode: [2]string{"single_image", "none"}},
		{name: "last frame", content: `[{"type":"image_url","role":"last_frame","image_url":{"url":"https://example.com/l.png"}}]`, expectedMode: [2]string{"single_image", "end_frame"}},
		{name: "reference image", content: `[{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/r.png"}}]`, expectedMode: [2]string{"multi_image", "reference"}},
		{name: "reference video", content: `[{"type":"video_url","video_url":{"url":"https://example.com/v.mp4"}}]`, expectedMode: [2]string{"multi_image", "reference"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			north := `{"model":"customer","duration":5,"resolution":"720p","content":` + testCase.content + `}`
			var payload requestPayload
			require.NoError(t, common.Unmarshal([]byte(north), &payload))
			var request map[string]any
			require.NoError(t, common.Unmarshal([]byte(north), &request))

			result, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{"tokensave_media_task_v1", "buildCreate"}, map[string]any{
				"protocol":      "tokensave_media_task_v1",
				"providerModel": providerModel,
				"request":       request,
				"limits":        map[string]any{"maxDurationSeconds": relaycommon.MaxTaskDurationSeconds},
			})
			require.NoError(t, err)
			object := result.(map[string]any)
			conversion, err := decodeMediaPluginCreate(object, request, providerModel, dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, providerModelSpec{}, &payload)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, common.Unmarshal(conversion.body, &wire))
			require.Equal(t, testCase.expectedMode[0], wire["input_mode"])
			require.Equal(t, testCase.expectedMode[1], wire["control_mode"])

			for _, field := range []string{"input_mode", "control_mode"} {
				var fullWire map[string]any
				require.NoError(t, common.Unmarshal(conversion.body, &fullWire))
				if field == "input_mode" {
					if testCase.expectedMode[0] == "text" {
						fullWire["input_mode"] = "single_image"
					} else {
						fullWire["input_mode"] = "text"
					}
				} else {
					if testCase.expectedMode[1] == "none" {
						fullWire["control_mode"] = "reference"
					} else {
						fullWire["control_mode"] = "none"
					}
				}
				tampered := map[string]any{"body": fullWire, "probe": map[string]any{}}
				_, err = decodeMediaPluginCreate(tampered, request, providerModel, dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, providerModelSpec{}, &payload)
				require.ErrorContains(t, err, "priced billing modes", "a drifted %s must be rejected before hold", field)
			}
		})
	}
}
