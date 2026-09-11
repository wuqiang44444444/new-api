package publicmodel

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedancePluginMetadataPreservesPublishedModelArkContract(t *testing.T) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	for _, video := range info.Configuration.Videos {
		models := append([]string(nil), video.Models...)
		if video.ModelPolicy == "configured" {
			models = append(models, "ep-explicit-customer-deployment")
		}
		for _, model := range models {
			t.Run(video.Protocol+"/"+model, func(t *testing.T) {
				spec, exists := video.ModelMetadata[model]
				if !exists {
					require.NotNil(t, video.DefaultModelMetadata)
					spec = *video.DefaultModelMetadata
				}
				expected, ok := VideoAPI("customer-model", dto.VideoUpstreamProtocol(video.Protocol), model, true)
				require.True(t, ok)
				actual, ok := VideoAPIFromPlugin("customer-model", dto.VideoUpstreamProtocol(video.Protocol), spec, true)
				require.True(t, ok)
				assert.Equal(t, expected, actual)
				assert.Equal(t, "customer-model", actual.Creation.Model)
			})
		}
	}
}
