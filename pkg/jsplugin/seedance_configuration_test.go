package jsplugin

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seedanceConfigurationFixture = `{
  videos: [{protocol: "feicai_videos_v1", label: "Test video", models: ["provider-one"],
    modelMetadata: {"provider-one": {minDuration: 4, maxDuration: 15, resolutions: ["720p"], ratios: ["16:9", "9:16"]}},
    assetProtocols: ["test_assets", "none"], defaultAssetProtocol: "test_assets"}],
  assets: [
    {protocol: "test_assets", label: "Test assets", groupPolicy: "default_fallback",
      credential: "asset_key_pair", defaultURLTTLSeconds: 3600,
      project: {required: true}, region: {required: true, fixed: "ap-southeast-1", format: "region_id"}},
    {protocol: "none", label: "No assets", groupPolicy: "none", credential: "none"}
  ]
}`

func seedanceConfigurationSource(configuration string) string {
	source := strings.Replace(seedanceValidSource, "apiVersion: 1", "apiVersion: 2", 1)
	source += `export const seedanceAssets = {test_assets: {buildRequest() {}, parseResponse() {}}};`
	return strings.Replace(source, `seedanceProtocols: ["feicai_videos_v1"],`,
		`seedanceProtocols: ["feicai_videos_v1"], channelConfiguration: `+configuration+`,`, 1)
}

func TestSeedanceConfigurationCompileAndPinnedDeclaration(t *testing.T) {
	contract := seedanceTestContract()
	contract.AssetProtocols = []string{"test_assets"}
	source := seedanceConfigurationSource(seedanceConfigurationFixture)
	// Runtime mutation of exported JavaScript data cannot mutate the validated
	// declaration already held by the host for this exact compiled artifact.
	source += `export function mutateConfiguration() { meta.channelConfiguration.videos[0].models[0] = "changed"; }`
	plugin, info, err := CompileSeedanceExtension(source, Options{}, contract)
	require.NoError(t, err)
	require.NotNil(t, info.Configuration)
	assert.Equal(t, 2, plugin.Meta.APIVersion)
	assert.Equal(t, []string{"provider-one"}, info.Configuration.Videos[0].Models)
	assert.Equal(t, "asset_key_pair", info.Configuration.Assets[0].Credential)
	assert.Equal(t, int64(3600), *info.Configuration.Assets[0].DefaultURLTTLSeconds)
	_, err = plugin.Engine.Call(context.Background(), "mutateConfiguration", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"provider-one"}, info.Configuration.Videos[0].Models)
	assert.Empty(t, plugin.Meta.Models, "Provider configuration must not become native model eligibility")
	assert.Empty(t, plugin.Meta.ChannelTypes)
	_, err = NewRegistry().Register(source, Options{})
	require.Error(t, err, "a configuration extension must not enter the native routing registry")

	_, history, err := CompileSeedanceExtension(seedanceValidSource, Options{}, contract)
	require.NoError(t, err, "historical v1 keeps its conversion contract")
	assert.Nil(t, history.Configuration)
}

func TestSeedanceConfigurationRejectsInvalidDeclarations(t *testing.T) {
	contract := seedanceTestContract()
	contract.AssetProtocols = []string{"test_assets"}
	cases := []struct{ name, from, to, errorText string }{
		{"missing declaration", seedanceConfigurationFixture, "null", "must be an object"},
		{"native routing injection", "videos: [", `models: ["native-model"], videos: [`, "unknown field"},
		{"secret value injection", `credential: "asset_key_pair"`, `credential: "asset_key_pair", secret: "not-a-real-secret"`, "unknown field"},
		{"arbitrary auth slot", `credential: "asset_key_pair"`, `credential: "custom_auth"`, "credential slot"},
		{"arbitrary field", `project: {required: true}`, `project: {required: true, script: "value"}`, "unknown field"},
		{"unimplemented asset operation", `protocol: "test_assets"`, `protocol: "missing_assets"`, "unsupported asset configuration"},
		{"undeclared pairing", `assetProtocols: ["test_assets", "none"]`, `assetProtocols: ["unknown"]`, "unknown or duplicate asset pairing"},
		{"invalid default", `defaultAssetProtocol: "test_assets"`, `defaultAssetProtocol: "unknown"`, "default asset protocol"},
		{"duplicate models", `models: ["provider-one"]`, `models: ["provider-one", "provider-one"]`, "duplicate Provider model"},
		{"empty models", `models: ["provider-one"]`, `models: []`, "Provider models"},
		{"missing protocol configuration", `videos: [{`, `videos: [{`, ""},
		{"null required", `required: true`, `required: null`, "must not be null"},
		{"string required", `required: true`, `required: "true"`, "field type"},
		{"negative TTL", `defaultURLTTLSeconds: 3600`, `defaultURLTTLSeconds: -1`, "positive URL TTL"},
		{"fractional TTL", `defaultURLTTLSeconds: 3600`, `defaultURLTTLSeconds: 0.5`, "field type"},
		{"invalid region", `fixed: "ap-southeast-1"`, `fixed: "not_a_region"`, "invalid configuration text"},
		{"conflicting default", `fixed: "ap-southeast-1"`, `fixed: "ap-southeast-1", default: "ap-northeast-1"`, "conflicts with fixed"},
		{"invalid group policy", `groupPolicy: "default_fallback"`, `groupPolicy: "create_automatically"`, "unsupported asset group policy"},
		{"hosted pairing mismatch", `groupPolicy: "default_fallback"`, `groupPolicy: "hosted"`, "platform-hosted asset protocol"},
		{"none requires credential", `credential: "none"`, `credential: "channel"`, "cannot require Provider fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			localContract := contract
			if tc.name == "missing protocol configuration" {
				localContract.Protocols = append(append([]SeedanceExtensionProtocol(nil), contract.Protocols...), SeedanceExtensionProtocol{Name: "another_video", Hooks: []string{"buildCreate"}})
			}
			source := seedanceConfigurationSource(strings.Replace(seedanceConfigurationFixture, tc.from, tc.to, 1))
			if tc.name == "missing protocol configuration" {
				source = strings.Replace(source, `seedanceProtocols: ["feicai_videos_v1"]`, `seedanceProtocols: ["feicai_videos_v1", "another_video"]`, 1)
				source = strings.Replace(source, `export const seedance = {`, `export const seedance = { another_video: { buildCreate() {} },`, 1)
				tc.errorText = "every implemented video protocol"
			}
			_, _, err := CompileSeedanceExtension(source, Options{}, localContract)
			require.ErrorContains(t, err, tc.errorText)
		})
	}
	_, _, err := CompileSeedanceExtension(strings.Replace(seedanceConfigurationSource(seedanceConfigurationFixture), "apiVersion: 2", "apiVersion: 1", 1), Options{}, contract)
	require.ErrorContains(t, err, "requires apiVersion 2")
}

func TestSeedanceConfigurationRequiresCompleteMetadata(t *testing.T) {
	contract := seedanceTestContract()
	contract.AssetProtocols = []string{"test_assets"}

	configuration := func(models, metadata, defaultMetadata string) string {
		return seedanceConfigurationSource(`{
  videos: [{protocol: "feicai_videos_v1", label: "Test video", models: ` + models + `,
    modelPolicy: "configured",
    modelMetadata: {` + metadata + `},
    defaultModelMetadata: ` + defaultMetadata + `,
    assetProtocols: ["test_assets", "none"], defaultAssetProtocol: "test_assets"}],
  assets: [
    {protocol: "test_assets", label: "Test assets", groupPolicy: "default_fallback",
      credential: "asset_key_pair", defaultURLTTLSeconds: 3600},
    {protocol: "none", label: "No assets", groupPolicy: "none", credential: "none"}
  ]
}`)
	}
	listed := func(models, metadata string) string {
		return seedanceConfigurationSource(`{
  videos: [{protocol: "feicai_videos_v1", label: "Test video", models: ` + models + `,
    modelMetadata: {` + metadata + `},
    assetProtocols: ["test_assets", "none"], defaultAssetProtocol: "test_assets"}],
  assets: [
    {protocol: "test_assets", label: "Test assets", groupPolicy: "default_fallback",
      credential: "asset_key_pair", defaultURLTTLSeconds: 3600},
    {protocol: "none", label: "No assets", groupPolicy: "none", credential: "none"}
  ]
}`)
	}

	t.Run("listed model without metadata is rejected", func(t *testing.T) {
		_, _, err := CompileSeedanceExtension(listed(`["provider-one", "provider-two"]`,
			`"provider-one": {minDuration: 4, maxDuration: 15}`), Options{}, contract)
		require.ErrorContains(t, err, "listed Provider model")
	})

	t.Run("configured policy requires default metadata", func(t *testing.T) {
		source := seedanceConfigurationSource(`{
  videos: [{protocol: "feicai_videos_v1", label: "Test video", models: ["provider-one"],
    modelPolicy: "configured",
    modelMetadata: {"provider-one": {minDuration: 4, maxDuration: 15}},
    assetProtocols: ["test_assets", "none"], defaultAssetProtocol: "test_assets"}],
  assets: [
    {protocol: "test_assets", label: "Test assets", groupPolicy: "default_fallback",
      credential: "asset_key_pair", defaultURLTTLSeconds: 3600},
    {protocol: "none", label: "No assets", groupPolicy: "none", credential: "none"}
  ]
}`)
		_, _, err := CompileSeedanceExtension(source, Options{}, contract)
		require.ErrorContains(t, err, "require default model metadata")
	})

	t.Run("configured default with enumerated override compiles", func(t *testing.T) {
		_, info, err := CompileSeedanceExtension(
			configuration(`["provider-one"]`,
				`"provider-one": {minDuration: 4, maxDuration: 10}`,
				`{minDuration: 1, maxDuration: 20}`), Options{}, contract)
		require.NoError(t, err)
		require.NotNil(t, info.Configuration.Videos[0].DefaultModelMetadata)
		assert.Equal(t, 20, info.Configuration.Videos[0].DefaultModelMetadata.MaxDuration)
		assert.Equal(t, 10, info.Configuration.Videos[0].ModelMetadata["provider-one"].MaxDuration)
	})

	t.Run("invalid default bounds", func(t *testing.T) {
		_, _, err := CompileSeedanceExtension(
			configuration(`["provider-one"]`, `"provider-one": {minDuration: 4, maxDuration: 15}`,
				`{minDuration: 30, maxDuration: 10}`), Options{}, contract)
		require.ErrorContains(t, err, "invalid ModelArk metadata bounds")
	})

	cases := []struct{ name, metadata, errorText string }{
		{"default below minimum", `{minDuration: 10, maxDuration: 15, defaultDuration: 5}`, "below the declared minimum"},
		{"default above maximum", `{minDuration: 1, maxDuration: 4, defaultDuration: 5}`, "above the declared maximum"},
		{"audio default without capability", `{publishGenerateAudioDefault: true, defaultGenerateAudio: true}`, "published audio default"},
		{"video limits without capability", `{maxVideos: 3}`, "video media limits"},
		{"audio limits without capability", `{maxAudios: 3}`, "audio media limits"},
		{"required resolution without options", `{resolutionRequired: true}`, "required resolution"},
		{"required ratio without options", `{ratioRequired: true}`, "required ratio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := CompileSeedanceExtension(listed(`["provider-one"]`,
				`"provider-one": `+tc.metadata), Options{}, contract)
			require.ErrorContains(t, err, tc.errorText)
		})
	}
}

func TestSeedanceConfigurationDefaultsDoNotRewriteChannelValues(t *testing.T) {
	contract := seedanceTestContract()
	contract.AssetProtocols = []string{"test_assets"}
	_, info, err := CompileSeedanceExtension(seedanceConfigurationSource(seedanceConfigurationFixture), Options{}, contract)
	require.NoError(t, err)
	configuration := info.Configuration
	require.NotNil(t, configuration)
	// The suggested 3600-second default is not a new minimum: an explicitly
	// configured positive TTL keeps its original meaning.
	require.NoError(t, configuration.ValidateChannel("feicai_videos_v1", "test_assets", []string{"provider-one"}, "project", "ap-southeast-1", 5))
	require.ErrorContains(t, configuration.ValidateChannel("feicai_videos_v1", "test_assets", []string{"provider-one"}, "project", "ap-southeast-1", 0), "positive URL TTL")
	require.ErrorContains(t, configuration.ValidateChannel("feicai_videos_v1", "test_assets", []string{"provider-one"}, "project", "ap-northeast-1", 5), "fixed value")
	require.ErrorContains(t, configuration.ValidateChannel("feicai_videos_v1", "test_assets", []string{"provider-one"}, "", "ap-southeast-1", 5), "project is required")
	require.NoError(t, configuration.ValidateChannel("feicai_videos_v1", "none", []string{"provider-one"}, "", "", 0))
}
