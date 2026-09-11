package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialPluginConfigurationCommitsSeparateCredentialAndPreservesTenant(t *testing.T) {
	db := withSeedanceChannelDB(t)
	seedPublishedSeedanceTestArtifact(t)
	published, err := GetSeedancePluginConfiguration()
	require.NoError(t, err)
	channel := seedanceTestChannel("custom-deployment", common.ChannelStatusEnabled)
	channel.ModelMapping = common.GetPointer(`{"custom-deployment":"ep-explicit-deployment"}`)
	channel.SeedancePluginVersion = published.Version
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine, AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction, AssetProviderProject: "project-a", AssetRegion: "cn-beijing", AssetMinURLTTLSeconds: 5})
	require.NoError(t, PinSeedanceChannelConfigurationInput(channel))
	require.NoError(t, InsertChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{AccessKeyID: "fixture-access", SecretAccessKey: "fixture-secret"}))
	originalScope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	require.NoError(t, SaveChannelDefaultAssetGroup(channel.Id, "provider-group"))
	originalSettings := channel.OtherSettings
	require.NoError(t, db.AutoMigrate(&Task{}, &TaskCreateAttempt{}))
	source := strings.Replace(plugins.SeedanceSource(), `version: "`+published.Version+`"`, `version: "8.0.0"`, 1)
	next := TaskPlugin{Key: "seedance-link", Version: "8.0.0", APIVersion: 3, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true}
	require.NoError(t, SaveTaskPlugin(&next))
	require.NoError(t, ActivateTaskPlugin(next.Key, next.Version))
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, originalSettings, stored.OtherSettings)
	afterScope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, originalScope, afterScope)
	group, err := GetChannelDefaultAssetGroup(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "provider-group", group.ProviderGroupID)
	require.ErrorContains(t, PinSeedanceChannelConfigurationInput(channel), "reload the channel form")
	source = strings.Replace(source, `version: "8.0.0"`, `version: "8.1.0"`, 1)
	source = strings.Replace(source, `fixed: "cn-beijing"`, `fixed: "other-region"`, 1)
	incompatible := TaskPlugin{Key: next.Key, Version: "8.1.0", APIVersion: 3, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true}
	require.NoError(t, SaveTaskPlugin(&incompatible))
	require.ErrorContains(t, ActivateTaskPlugin(next.Key, incompatible.Version), "incompatible with channels")
	active, err := GetTaskPluginVersion(next.Key, "")
	require.NoError(t, err)
	assert.Equal(t, next.Version, active.Version)
}

func TestPluginAssetMetadataMatchesPublishedContract(t *testing.T) {
	db := withSeedanceChannelDB(t)
	_ = db
	seedPublishedSeedanceTestArtifact(t)
	published, err := GetSeedancePluginConfiguration()
	require.NoError(t, err)
	for _, definition := range published.Configuration.Assets {
		protocol := dto.AssetUpstreamProtocol(definition.Protocol)
		expected := seedancePublicAssetAPI("customer", protocol, 5, "scope")
		actual := seedancePublicAssetAPIWithDeclaration("customer", protocol, 5, "scope", &definition)
		assert.Equal(t, expected, actual)
	}
}
