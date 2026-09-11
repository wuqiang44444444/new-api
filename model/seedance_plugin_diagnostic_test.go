package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceConfigurationDiagnosticDoesNotExposeArtifactValues(t *testing.T) {
	for _, tc := range []struct {
		name, suffix, want string
	}{
		{"JavaScript exception", `throw new Error("private-artifact-marker");`, "invalid plugin code or declaration"},
		{"unknown metadata field", `meta.channelConfiguration["private-artifact-marker"] = true;`, "invalid plugin code or declaration"},
		{"invalid protocol", `meta.channelConfiguration.videos[0].protocol = "private-artifact-marker";`, "duplicate or unimplemented video configuration"},
		{"missing model metadata", `meta.channelConfiguration.videos[0].models = ["private-artifact-marker"];`, "listed Provider model requires declared model metadata"},
		{"inconsistent bounds", `meta.channelConfiguration.videos[0].modelMetadata["provider-one"].defaultDuration = 16;`, "default duration is above the declared maximum duration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifact := configurationTestPlugin("2.0.0", "provider-one")
			artifact.Source += tc.suffix
			_, err := seedanceConfigurationForArtifact(&artifact)
			require.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "private-artifact-marker")
		})
	}
}
