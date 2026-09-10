package plugins

import (
	_ "embed"
)

// seedanceLinkSource is the built-in Seedance Link extension artifact. Unlike
// the generic task plugins under tasks/, it is never registered into the
// native factory registry: it is compiled through the Seedance extension
// contract and seeded into the task_plugin version store, where pinned
// versions survive gateway upgrades.
//
//go:embed seedance-link/plugin.js
var seedanceLinkSource string

// SeedanceSource returns the built-in Seedance Link extension artifact.
func SeedanceSource() string {
	return seedanceLinkSource
}
