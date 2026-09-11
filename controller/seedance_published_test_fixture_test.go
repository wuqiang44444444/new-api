package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/require"
)

func seedPublishedSeedanceControllerArtifact(t *testing.T) string {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.TaskPlugin{}, &model.ChannelAssetCredential{}))
	source := plugins.SeedanceSource()
	plugin, _, err := jsplugin.CompileSeedanceExtension(source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	row := model.TaskPlugin{Key: plugin.Meta.Key, Version: plugin.Meta.Version, APIVersion: plugin.Meta.APIVersion, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true, Active: true}
	require.NoError(t, model.DB.Create(&row).Error)
	previous := seedanceplugin.Default
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() { seedanceplugin.Default = previous })
	require.NoError(t, seedanceplugin.Default.SyncSnapshot(context.Background(), []model.TaskPlugin{row}))
	return row.Version
}
