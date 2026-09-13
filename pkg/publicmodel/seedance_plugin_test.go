package publicmodel

import (
	"strings"
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
				if video.Protocol == string(dto.VideoUpstreamProtocolSynlinkVideoV1) ||
					((video.Protocol == string(dto.VideoUpstreamProtocolModelArkV3Volcengine) || video.Protocol == string(dto.VideoUpstreamProtocolModelArkV3BytePlus)) && strings.HasSuffix(model, "-seedance-2-5-260628")) {
					// These model limits were corrected against September's official
					// specs. Preserve endpoint semantics without freezing obsolete limits.
					assert.Equal(t, expected.Operations, actual.Operations)
					assert.Equal(t, expected.Protocol, actual.Protocol)
					assert.Equal(t, expected.Creation.RequiredFields, actual.Creation.RequiredFields)
					if video.Protocol != string(dto.VideoUpstreamProtocolSynlinkVideoV1) {
						for _, parameter := range actual.Creation.Parameters {
							if parameter.Name == "resolution" {
								assert.Equal(t, []string{"480p", "720p", "1080p"}, parameter.Enum)
							}
						}
					}
				} else {
					assert.Equal(t, expected, actual)
				}
				assert.Equal(t, "customer-model", actual.Creation.Model)
			})
		}
	}
}
