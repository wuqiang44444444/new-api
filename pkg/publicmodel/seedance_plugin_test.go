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
		if video.Protocol == string(dto.VideoUpstreamProtocolViduModelArkV3) {
			// Vidu has no legacy projection; its published contract is tested below.
			continue
		}
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

func TestViduPublishedContractMatchesBothRegions(t *testing.T) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	count := 0
	for _, video := range info.Configuration.Videos {
		if video.Protocol != string(dto.VideoUpstreamProtocolViduModelArkV3) {
			continue
		}
		for name, metadata := range video.ModelMetadata {
			count++
			api, ok := VideoAPIFromPlugin("customer-model", dto.VideoUpstreamProtocolViduModelArkV3, metadata, true)
			require.True(t, ok)
			parameters := map[string]dto.PublicAPIParameter{}
			for _, parameter := range api.Creation.Parameters {
				parameters[parameter.Name] = parameter
			}
			var fields []string
			for name := range parameters {
				fields = append(fields, name)
			}
			assert.ElementsMatch(t, []string{"model", "content", "resolution", "ratio", "duration", "generate_audio", "watermark", "callback_url", "return_last_frame", "execution_expires_after", "tools", "safety_identifier"}, fields, name)
			assert.Equal(t, "adaptive", parameters["ratio"].DefaultValue)
			assert.Equal(t, false, parameters["generate_audio"].DefaultValue)
			assert.Equal(t, 172800, parameters["execution_expires_after"].DefaultValue)
			require.NotNil(t, parameters["safety_identifier"].MaxLength)
			assert.Equal(t, 64, *parameters["safety_identifier"].MaxLength)
			assert.Equal(t, "customer-model", api.Creation.Model)
			for _, operation := range api.Operations {
				if operation.Operation == "delete_video" {
					assert.False(t, operation.Supported)
				}
			}
			if strings.HasPrefix(name, "viduq3.1-") {
				assert.Equal(t, 30, metadata.MaxDuration)
				assert.Equal(t, 30, metadata.MaxImages)
				assert.Equal(t, 10, metadata.MaxAudios)
			} else {
				assert.Equal(t, 15, metadata.MaxDuration)
			}
		}
	}
	assert.Equal(t, 8, count)
}
