package plugins

import (
	_ "embed"
	"regexp"
)

// minimaxLinkSource is the built-in MiniMax Link extension artifact. Like the
// Seedance artifact it is never registered into the native factory registry:
// it is compiled through the typed-extension contract and seeded into the
// task_plugin version store. Unlike Seedance, deployment only inserts the
// version — activation stays an explicit administrator action.
//
//go:embed minimax-link/plugin.js
var minimaxLinkSource string

var minimaxVersionPattern = regexp.MustCompile(`version: "([^"]+)"`)

// MinimaxVersion returns the built-in artifact's declared version, so tests
// and tooling never pin a stale version label.
func MinimaxVersion() string {
	if match := minimaxVersionPattern.FindStringSubmatch(minimaxLinkSource); match != nil {
		return match[1]
	}
	return ""
}

// MinimaxSource returns the built-in MiniMax Link extension artifact.
func MinimaxSource() string {
	return minimaxLinkSource
}
