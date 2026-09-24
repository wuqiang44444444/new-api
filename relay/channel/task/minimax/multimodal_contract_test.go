package minimax

import (
	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMiniMaxMultimodalPreservationAndHostGuards(t *testing.T) {
	plugin, info, err := pluginruntime.CompileSeedanceExtension(plugins.MinimaxSource(), pluginruntime.Options{}, pluginruntime.MinimaxHostContract())
	require.NoError(t, err)
	request := textRequest("prompt")
	request["content"] = append(request["content"].([]any), map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "data:image/png;base64,YQ=="}})
	for _, duration := range []int{4, 15} {
		for _, ratio := range []string{"16:9", "21:9", "4:3", "1:1", "3:4", "9:16", "adaptive"} {
			request["duration"] = duration
			request["ratio"] = ratio
			result, err := callBuildCreate(t, plugin, request, "")
			require.NoError(t, err)
			raw, err := common.Marshal(request)
			require.NoError(t, err)
			var contract taskdto.ModelArkVideoCreateRequest
			require.NoError(t, common.Unmarshal(raw, &contract))
			_, err = decodeCreateConversion(result, info.Configuration, "MiniMax-H3", &contract)
			require.NoError(t, err)
			body := result.(map[string]any)["body"].(map[string]any)
			body["content"].([]any)[1].(map[string]any)["role"] = "first_frame"
			_, err = decodeCreateConversion(result, info.Configuration, "MiniMax-H3", &contract)
			require.Error(t, err, "host must reject role reinterpretation")
			body["content"].([]any)[1].(map[string]any)["role"] = "reference_image"
		}
	}
	request = textRequest("prompt")
	result, err := callBuildCreate(t, plugin, request, "")
	require.NoError(t, err)
	result.(map[string]any)["body"].(map[string]any)["parameters"].(map[string]any)["resolution"] = "2K"
	contract := &taskdto.ModelArkVideoCreateRequest{Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("prompt")}}}
	_, err = decodeCreateConversion(result, info.Configuration, "MiniMax-H3", contract)
	assert.Error(t, err, "default wire resolution must agree with the billing probe")
}
