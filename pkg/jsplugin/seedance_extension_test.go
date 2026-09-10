package jsplugin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedanceTestContract() SeedanceExtensionContract {
	return SeedanceExtensionContract{
		Key: "seedance-link",
		Protocols: []SeedanceExtensionProtocol{
			{Name: "feicai_videos_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
		},
	}
}

const seedanceValidSource = `
export const meta = {
  apiVersion: 1,
  key: "seedance-link",
  name: "Seedance Link",
  version: "1.0.0",
  author: { name: "dev" },
  seedanceProtocols: ["feicai_videos_v1"],
};
export const seedance = {
  "feicai_videos_v1": {
    buildCreate() { return null; },
    parseCreateResponse() { return null; },
    parseTaskObservation() { return null; },
  },
};
`

func TestCompileSeedanceExtensionAcceptsValidArtifact(t *testing.T) {
	plugin, info, err := CompileSeedanceExtension(seedanceValidSource, Options{Key: "seedance-link"}, seedanceTestContract())
	require.NoError(t, err)
	assert.Equal(t, "seedance-link", plugin.Meta.Key)
	assert.Equal(t, "1.0.0", plugin.Meta.Version)
	assert.Equal(t, []string{"feicai_videos_v1"}, info.Protocols)
	assert.Empty(t, plugin.Meta.Models)
	assert.Empty(t, plugin.Meta.ChannelTypes)
	assert.Empty(t, plugin.Meta.Routes)
	assert.Empty(t, plugin.Meta.Protocols)
	assert.Empty(t, plugin.Meta.UsageSchema)
	assert.Equal(t, "seedance-link", plugin.Engine.key)
	assert.Equal(t, "1.0.0", plugin.Engine.version)
}

func TestCompileSeedanceExtensionNeverEntersDefaultRegistry(t *testing.T) {
	before := DefaultRegistry.Snapshot()
	generationBefore := DefaultRegistry.Generation()

	_, _, err := CompileSeedanceExtension(seedanceValidSource, Options{Key: "seedance-link"}, seedanceTestContract())
	require.NoError(t, err)

	assert.Equal(t, before, DefaultRegistry.Snapshot())
	assert.Same(t, generationBefore, DefaultRegistry.Generation())
	_, byModel := DefaultRegistry.Generation().GetByModel("seedance-link")
	assert.False(t, byModel)
	_, ok := DefaultRegistry.Generation().Get("seedance-link")
	assert.False(t, ok)
}

func TestCompileSeedanceExtensionRejections(t *testing.T) {
	contract := seedanceTestContract()
	cases := []struct {
		name   string
		source string
		expect string
	}{
		{
			name: "native routing field models",
			source: strings.Replace(seedanceValidSource,
				`seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["feicai_videos_v1"], models: ["some-model"],`, 1),
			expect: `unknown field "models"`,
		},
		{
			name: "native routing field channelTypes",
			source: strings.Replace(seedanceValidSource,
				`seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["feicai_videos_v1"], channelTypes: [62],`, 1),
			expect: `unknown field "channelTypes"`,
		},
		{
			name: "usage schema field",
			source: strings.Replace(seedanceValidSource,
				`seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["feicai_videos_v1"], usageSchema: {tokens: {type: "number", unit: "token"}},`, 1),
			expect: `unknown field "usageSchema"`,
		},
		{
			name: "auth field",
			source: strings.Replace(seedanceValidSource,
				`seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["feicai_videos_v1"], auth: {type: "api_key"},`, 1),
			expect: `unknown field "auth"`,
		},
		{
			name:   "wrong key",
			source: strings.Replace(seedanceValidSource, `key: "seedance-link",`, `key: "other-link",`, 1),
			expect: `meta key must be "seedance-link"`,
		},
		{
			name:   "bad version",
			source: strings.Replace(seedanceValidSource, `version: "1.0.0",`, `version: "latest",`, 1),
			expect: "not a semantic version",
		},
		{
			name:   "empty protocol list",
			source: strings.Replace(seedanceValidSource, `seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: [],`, 1),
			expect: "at least one protocol",
		},
		{
			name:   "unregistered protocol",
			source: strings.Replace(seedanceValidSource, `seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["moxing_media_task_v1"],`, 1),
			expect: `unregistered protocol "moxing_media_task_v1"`,
		},
		{
			name:   "duplicate protocol",
			source: strings.Replace(seedanceValidSource, `seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["feicai_videos_v1", "feicai_videos_v1"],`, 1),
			expect: `declares protocol "feicai_videos_v1" twice`,
		},
		{
			name: "missing hook",
			source: strings.Replace(seedanceValidSource,
				`parseTaskObservation() { return null; },`, ``, 1),
			expect: `missing required export seedance.feicai_videos_v1.parseTaskObservation`,
		},
		{
			name: "unsupported member",
			source: strings.Replace(seedanceValidSource,
				`parseTaskObservation() { return null; },`, `parseTaskObservation() { return null; }, buildQuery() { return null; },`, 1),
			expect: `unsupported member "buildQuery"`,
		},
		{
			name: "undeclared implemented protocol",
			source: strings.Replace(seedanceValidSource,
				`seedanceProtocols: ["feicai_videos_v1"],`, `seedanceProtocols: ["feicai_videos_v1", "modelark_v3_volcengine"],`, 1),
			expect: `unregistered protocol "modelark_v3_volcengine"`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := CompileSeedanceExtension(testCase.source, Options{Key: "seedance-link"}, contract)
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.expect)
		})
	}
}

func TestCompileSeedanceExtensionRejectsNonObjectRoot(t *testing.T) {
	source := strings.Replace(seedanceValidSource,
		`export const seedance = {
  "feicai_videos_v1": {
    buildCreate() { return null; },
    parseCreateResponse() { return null; },
    parseTaskObservation() { return null; },
  },
};`, `export const seedance = () => ({});`, 1)
	_, _, err := CompileSeedanceExtension(source, Options{Key: "seedance-link"}, seedanceTestContract())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "export seedance must be an object")
}

func TestCompileSeedanceExtensionContractValidation(t *testing.T) {
	_, _, err := CompileSeedanceExtension(seedanceValidSource, Options{Key: "seedance-link"}, SeedanceExtensionContract{
		Key:       "seedance-link",
		Protocols: []SeedanceExtensionProtocol{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no protocols")

	_, _, err = CompileSeedanceExtension(seedanceValidSource, Options{Key: "seedance-link"}, SeedanceExtensionContract{
		Key: "bad key!",
		Protocols: []SeedanceExtensionProtocol{
			{Name: "feicai_videos_v1", Hooks: []string{"buildCreate"}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract key")
}
