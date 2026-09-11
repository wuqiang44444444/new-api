package main

import (
	"fmt"
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"gorm.io/gorm"
)

type seedanceArtifactRow struct {
	Key        string
	Version    string
	APIVersion int
	Source     string
	Active     bool
	Enabled    bool
}

// Inspect every version, including disabled and inactive artifacts: historical
// tasks may still resolve them. This does not activate, seed or rewrite sources.
func checkSeedancePluginArtifacts(db *gorm.DB, output io.Writer) error {
	var rows []seedanceArtifactRow
	if err := db.Table("task_plugins").
		Select("key", "version", "api_version", "source", "active", "enabled").
		Where("key = ?", jsplugin.SeedancePluginKey).Order("version").Find(&rows).Error; err != nil {
		return fmt.Errorf("cannot read stored Seedance artifacts")
	}
	rejected := 0
	for _, row := range rows {
		loaded, _, err := jsplugin.CompileSeedanceExtension(row.Source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
		result := "compatible"
		if err != nil {
			result = jsplugin.SeedanceCompilationDiagnostic(err)
			rejected++
		} else if loaded.Meta.Key != row.Key || loaded.Meta.Version != row.Version || loaded.Meta.APIVersion != row.APIVersion {
			result = "plugin identity does not match its stored artifact"
			rejected++
		}
		if _, err := fmt.Fprintf(output, "version=%q api=%d active=%t enabled=%t source_sha256=%x result=%s\n",
			row.Version, row.APIVersion, row.Active, row.Enabled, common.Sha256Raw([]byte(row.Source)), result); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "checked=%d rejected=%d\n", len(rows), rejected); err != nil {
		return err
	}
	if rejected > 0 {
		return fmt.Errorf("%d stored Seedance artifact(s) require review before upgrade", rejected)
	}
	return nil
}
