package publicmodel

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImageRelayPublishesEditLimitsForEveryRegisteredModel(t *testing.T) {
	for _, protocol := range []dto.ImageUpstreamProtocol{dto.ImageUpstreamProtocolFunCloudAIGCV2, dto.ImageUpstreamProtocolMoxingImagesV1} {
		for _, model := range constant.ImageRelayProviderModels(protocol) {
			api, ok := ImageAPI("customer-model", protocol, model)
			require.True(t, ok)
			require.NotNil(t, api.Image.Edit)
			assert.Equal(t, "/v1/images/edits", api.Image.Edit.Path)
			assert.Equal(t, []string{"image", "images"}, api.Image.Edit.RequiredOneOf)
			limit, _, _ := constant.ImageRelayInputLimits(protocol, model)
			found := false
			for _, parameter := range api.Image.Edit.Parameters {
				if parameter.Name == "images" {
					found = true
					require.NotNil(t, parameter.MaxItems)
					assert.Equal(t, limit, *parameter.MaxItems)
				}
			}
			assert.True(t, found)
			require.Len(t, api.Image.Operations, 2)
			assert.True(t, api.Image.Operations[1].Supported)
		}
	}
}
