package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestArtifactPreflightChecksInactiveVersionsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.Table("task_plugins").AutoMigrate(&seedanceArtifactRow{}))
	rows := []seedanceArtifactRow{
		{Key: jsplugin.SeedancePluginKey, Version: plugins.SeedanceVersion(), APIVersion: 3,
			Source: plugins.SeedanceSource(), Active: true, Enabled: true},
		{Key: jsplugin.SeedancePluginKey, Version: "0.9.0", APIVersion: 2, Source: `
export const meta = {apiVersion: 2, key: "seedance-link", name: "Test", version: "0.9.0",
 author: {name: "test"}, seedanceProtocols: ["feicai_videos_v1"], channelConfiguration: {
 videos: [{protocol: "feicai_videos_v1", label: "Video", models: ["private-artifact-marker"], assetProtocols: ["none"], defaultAssetProtocol: "none"}],
 assets: [{protocol: "none", label: "None", groupPolicy: "none", credential: "none"}]
 }};
export const seedance = {feicai_videos_v1: {buildCreate() {}, parseCreateResponse() {}, parseTaskObservation() {}}};`},
		{Key: jsplugin.SeedancePluginKey, Version: "0.8.0", APIVersion: 2, Source: `throw new Error("private-artifact-marker");`},
		{Key: jsplugin.SeedancePluginKey, Version: "0.7.0", APIVersion: 3, Source: plugins.SeedanceSource()},
		{Key: "unrelated", Version: "1.0.0", Source: "not a Seedance artifact"},
	}
	require.NoError(t, db.Table("task_plugins").Create(&rows).Error)
	readOnly, err := gorm.Open(sqlite.Open(path+"?mode=ro"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	readDB, err := readOnly.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readDB.Close()) })

	var output bytes.Buffer
	require.ErrorContains(t, checkSeedancePluginArtifacts(readOnly, &output), "3 stored Seedance artifact(s) require review")
	assert.Contains(t, output.String(), "checked=4 rejected=3")
	assert.Contains(t, output.String(), "listed Provider model requires declared model metadata")
	assert.Contains(t, output.String(), "invalid plugin code or declaration")
	assert.Contains(t, output.String(), "plugin identity does not match its stored artifact")
	assert.Contains(t, output.String(), "active=false enabled=false")
	assert.NotContains(t, output.String(), "private-artifact-marker")
	assert.NotContains(t, output.String(), "export const")
	var persisted []seedanceArtifactRow
	require.NoError(t, readOnly.Table("task_plugins").Find(&persisted).Error)
	assert.Equal(t, rows, persisted, "preflight must preserve all sources and activation flags")

	// Once only the valid artifact remains, the same read-only check succeeds.
	require.NoError(t, db.Table("task_plugins").Where("version IN ?", []string{"0.9.0", "0.8.0", "0.7.0"}).Delete(&seedanceArtifactRow{}).Error)
	output.Reset()
	require.NoError(t, checkSeedancePluginArtifacts(readOnly, &output))
	assert.True(t, strings.HasSuffix(output.String(), "checked=1 rejected=0\n"))
}
