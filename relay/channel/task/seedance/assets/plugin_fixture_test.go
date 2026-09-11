package assets

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/require"
	"testing"
)

func publishedAssetFixture(t *testing.T, protocol dto.AssetUpstreamProtocol, base, key string, client HTTPDoer) *PluginAssetAdapter {
	t.Helper()
	plugin, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	adapter, err := NewPluginAssetAdapter(plugin, info.Configuration.Asset(string(protocol)), protocol, base, key, "", "", client)
	require.NoError(t, err)
	return adapter
}
func publishedCMCCFixture(t *testing.T, key string, client HTTPDoer) (*PluginAssetAdapter, error) {
	return publishedAssetFixture(t, dto.AssetUpstreamProtocolCMCCAICCV2, "", key, client), nil
}
