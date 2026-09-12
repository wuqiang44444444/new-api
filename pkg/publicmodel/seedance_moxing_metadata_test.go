package publicmodel

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoxingPublishedResolutionAndFormatFollowModelDocumentation(t *testing.T) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	var models map[string]jsplugin.SeedanceVideoModelMetadata
	for _, video := range info.Configuration.Videos {
		if video.Protocol == "moxing_modelark_media_v1" {
			models = video.ModelMetadata
		}
	}
	require.Len(t, models, 4)
	for _, tc := range []struct {
		model           string
		enum, suggested []string
		outputFormat    bool
	}{
		{"doubao-seedance-2-0-260128-0818", []string{"480p", "720p", "1080p", "4k"}, nil, false},
		{"doubao-seedance-2-5-260628", []string{"480p", "720p"}, nil, true},
		{"doubao-seedance-2-0-mini-260615", nil, []string{"480p", "720p"}, false},
		{"doubao-seedance-2-0-fast-260128", nil, []string{"480p", "720p"}, false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			api, ok := VideoAPIFromPlugin("customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, models[tc.model], false)
			require.True(t, ok)
			parameters := map[string]dto.PublicAPIParameter{}
			for _, parameter := range api.Creation.Parameters {
				require.NotContains(t, parameters, parameter.Name)
				parameters[parameter.Name] = parameter
			}
			require.Contains(t, parameters, "resolution")
			assert.Equal(t, tc.enum, parameters["resolution"].Enum)
			assert.Equal(t, tc.suggested, parameters["resolution"].SuggestedValues)
			assert.Equal(t, "720p", parameters["resolution"].DefaultValue)
			_, exists := parameters["output_format"]
			assert.Equal(t, tc.outputFormat, exists)
			if tc.outputFormat {
				assert.Equal(t, []string{"mp4", "mov"}, parameters["output_format"].Enum)
			}
		})
	}
}
