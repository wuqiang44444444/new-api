package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/require"
)

func seedPublishedSeedanceTestArtifact(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&TaskPlugin{}, &ChannelAssetCredential{}))
	source := plugins.SeedanceSource()
	plugin, _, err := jsplugin.CompileSeedanceExtension(source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	require.NoError(t, DB.Create(&TaskPlugin{Key: plugin.Meta.Key, Version: plugin.Meta.Version, APIVersion: plugin.Meta.APIVersion, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true, Active: true}).Error)
}
