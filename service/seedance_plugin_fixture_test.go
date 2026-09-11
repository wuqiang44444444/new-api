package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/require"
)

func pinPublishedSeedanceServiceArtifact(t *testing.T) {
	t.Helper()
	source := plugins.SeedanceSource()
	loaded, _, err := jsplugin.CompileSeedanceExtension(source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	previous := seedanceplugin.Default
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() { seedanceplugin.Default = previous })
	require.NoError(t, seedanceplugin.Default.SyncSnapshot(context.Background(), []model.TaskPlugin{{Key: loaded.Meta.Key, Version: loaded.Meta.Version, APIVersion: loaded.Meta.APIVersion, Source: source, Enabled: true, Active: true}}))
}
