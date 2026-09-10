package publicmodel

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReferenceMediaCatalogMatchesTransport(t *testing.T) {
	for _, tt := range []struct {
		protocol dto.VideoUpstreamProtocol
		model    string
	}{
		{dto.VideoUpstreamProtocolModelArkV3CMCC, "doubao-seedance-2.0"},
		{dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, "doubao-seedance-2-0-260128"},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-0"},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-0-fast"},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-0-mini"},
		{dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-5"},
	} {
		t.Run(string(tt.protocol)+"/"+tt.model, func(t *testing.T) {
			api, ok := VideoAPI("customer", tt.protocol, tt.model, false)
			require.True(t, ok)
			require.Len(t, api.Creation.ContentTypes, 4)
			if tt.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
				assert.Equal(t, 30, api.Creation.ContentTypes[1].MaxItems)
				assert.Equal(t, 10, api.Creation.ContentTypes[2].MaxItems)
				assert.Equal(t, 10, api.Creation.ContentTypes[3].MaxItems)
			}
		})
	}
	for _, name := range []string{"seedance-2", "seedance-2-fast", "seedance-2-mini", "seedance-2-5"} {
		_, ok := VideoAPI("customer", dto.VideoUpstreamProtocolFunCloudSeedance, name, false)
		assert.False(t, ok)
	}
}
