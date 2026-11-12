package publicmodel

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexhinaSeedancePublishesVerifiedMediaAndOperations(t *testing.T) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	const providerModel = "doubao-seedance-2-0-mini-260615"
	for _, video := range info.Configuration.Videos {
		if video.Protocol != string(dto.VideoUpstreamProtocolNexhinaModelArkV3) {
			continue
		}
		metadata, exists := video.ModelMetadata[providerModel]
		require.True(t, exists)
		api, ok := VideoAPIFromPlugin("customer-mini", dto.VideoUpstreamProtocolNexhinaModelArkV3, metadata, false)
		require.True(t, ok)
		assert.Equal(t, "modelark_v3", api.Protocol)
		assert.Equal(t, "/api/v3/contents/generations/tasks", api.Creation.Path)
		assert.Equal(t, "customer-mini", api.Creation.Model)
		require.Len(t, api.Creation.ContentTypes, 2)
		assert.Equal(t, "text", api.Creation.ContentTypes[0].Type)
		assert.Equal(t, "image_url", api.Creation.ContentTypes[1].Type)
		assert.Contains(t, api.Creation.ContentTypes[1].Roles, "reference_image")
		assert.Equal(t, 1, api.Creation.ContentTypes[1].MaxItems)
		assert.Contains(t, video.AssetProtocols, string(dto.AssetUpstreamProtocolNexhinaArkV1))
		operations := map[string]bool{}
		for _, operation := range api.Operations {
			operations[operation.Operation] = operation.Supported
		}
		require.Contains(t, operations, "delete_video")
		assert.False(t, operations["delete_video"])
		assert.True(t, operations["get_video_content"])
		return
	}
	t.Fatal("Nexhina video declaration is missing")
}
