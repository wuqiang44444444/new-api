package plugins

import (
	_ "embed"
	"regexp"
)

// seedanceLinkSource is the built-in Seedance Link extension artifact. Unlike
// the generic task plugins under tasks/, it is never registered into the
// native factory registry: it is compiled through the Seedance extension
// contract and seeded into the task_plugin version store, where pinned
// versions survive gateway upgrades.
//
//go:embed seedance-link/plugin.js
var seedanceLinkSource string

var seedanceVersionPattern = regexp.MustCompile(`version: "([^"]+)"`)

// SeedanceVersion returns the built-in artifact's declared version, so tests
// and tooling never pin a stale version label.
func SeedanceVersion() string {
	if match := seedanceVersionPattern.FindStringSubmatch(seedanceLinkSource); match != nil {
		return match[1]
	}
	return ""
}

// SeedanceSource returns the built-in Seedance Link extension artifact.
func SeedanceSource() string {
	return seedanceLinkSource
}
