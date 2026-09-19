package publicmodel

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskMetadataPublishesExecutionAndDeliveryContract(t *testing.T) {
	apis := map[string]*dto.PublicModelAPI{
		"gemini": GeminiImageAPI("customer-image", "gemini-3.1-flash-image", constant.ChannelTypeGemini),
		"vertex": GeminiImageAPI("customer-image", "gemini-3.1-flash-lite-image", constant.ChannelTypeVertexAi),
	}
	for _, protocol := range []dto.ImageUpstreamProtocol{dto.ImageUpstreamProtocolFunCloudAIGCV2, dto.ImageUpstreamProtocolMoxingImagesV1} {
		for _, providerModel := range constant.ImageRelayProviderModels(protocol) {
			api, ok := ImageAPI("customer-image", protocol, providerModel)
			require.True(t, ok)
			apis[string(protocol)+"/"+providerModel] = api
		}
	}
	for name, api := range apis {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, api.Image.Async)
			assert.Equal(t, "Prefer", api.Image.Async.RequestHeader)
			assert.Equal(t, "respond-async", api.Image.Async.RequestValue)
			assert.Equal(t, "/v1/tasks/{task_id}", api.Image.Async.QueryPath)
			assert.False(t, api.Image.Async.StreamPriority)
			assert.Contains(t, api.Image.Operations, dto.PublicAPIOperation{
				Operation: "query_image", Method: http.MethodGet, Path: "/v1/tasks/{task_id}", Supported: true,
			})
			require.NotNil(t, api.Image.Edit)
			for _, operation := range []dto.PublicImageCreation{api.Image.Creation, *api.Image.Edit} {
				found := false
				for _, parameter := range operation.Parameters {
					if parameter.Name == "response_format" {
						found = true
						assert.ElementsMatch(t, []string{"url", "b64_json"}, parameter.Enum)
					}
				}
				assert.True(t, found, "generation and editing must publish the response format selector")
			}
		})
	}
}
