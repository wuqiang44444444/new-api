package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkModelLimitsMatchDiscoveryAndAdmission(t *testing.T) {
	for _, tc := range []struct {
		model       string
		resolutions []string
		maximum     int
	}{
		{"doubao-seedance-2-0-260128", []string{"480p", "720p", "1080p", "4k"}, 15},
		{"doubao-seedance-2-0-fast-260128", []string{"480p", "720p"}, 15},
		{"doubao-seedance-2-0-mini-260615", []string{"480p", "720p"}, 15},
		{"doubao-seedance-2-5-260628", []string{"480p", "720p", "1080p"}, 30},
	} {
		t.Run(tc.model, func(t *testing.T) {
			api, ok := publishedVideoFixture("customer-s", kitdto.VideoUpstreamProtocolSynlinkVideoV1, tc.model, false)
			require.True(t, ok)
			params := map[string]kitdto.PublicAPIParameter{}
			for _, p := range api.Creation.Parameters {
				params[p.Name] = p
			}
			assert.Equal(t, tc.resolutions, params["resolution"].Enum)
			require.NotNil(t, params["duration"].Maximum)
			assert.Equal(t, tc.maximum, *params["duration"].Maximum)
			for _, resolution := range []string{"480p", "720p", "1080p", "4k"} {
				for _, duration := range []int{3, 4, tc.maximum, tc.maximum + 1} {
					req := providerTestRequest()
					req.Resolution, req.Duration = &resolution, &duration
					c := seedancePluginTestContext(t)
					pinSeedanceExtensionForTest(t, c)
					relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
					a := &TaskAdaptor{protocol: kitdto.VideoUpstreamProtocolSynlinkVideoV1, baseURL: "https://provider.example"}
					_, err := a.ensureSeedanceCreateConversion(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: tc.model, ChannelBaseUrl: "https://provider.example"}})
					supported := false
					for _, value := range tc.resolutions {
						supported = supported || resolution == value
					}
					if supported && duration >= 4 && duration <= tc.maximum {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				}
			}
			assert.Equal(t, false, params["generate_audio"].DefaultValue)
			assert.Equal(t, common.GetPointer(4), params["duration"].Minimum)
		})
	}
}
