package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func officialAssetTestChannel(t *testing.T, id int) *Channel {
	t.Helper()
	channel := &Channel{
		Id:     id,
		Type:   constant.ChannelTypeDoubaoVideo,
		Key:    "video-api-key",
		Status: common.ChannelStatusEnabled,
		Name:   "official-assets",
		Models: "video-model",
		Group:  "default",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:  dto.VideoUpstreamProfileOfficial,
		AssetUpstreamProfile:  dto.AssetUpstreamProfileOfficial,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "project-a",
		AssetRegion:           "ap-southeast-1",
	})
	return channel
}

func TestMaskAssetAccessKeyID(t *testing.T) {
	assert.Equal(t, "AK******YAA", MaskAssetAccessKeyID("  AK123456YAA  "))
	assert.Equal(t, "*****", MaskAssetAccessKeyID("short"))
	assert.Empty(t, MaskAssetAccessKeyID(" "))
}

func TestOfficialAssetCredentialFingerprintUsesActionIdentity(t *testing.T) {
	base := OfficialAssetCredentialFingerprint("access", "secret", "project-a", "ap-southeast-1")
	assert.Equal(t, base, OfficialAssetCredentialFingerprint(" access ", " secret ", " project-a ", " ap-southeast-1 "))
	assert.NotEqual(t, base, OfficialAssetCredentialFingerprint("other", "secret", "project-a", "ap-southeast-1"))
	assert.NotEqual(t, base, OfficialAssetCredentialFingerprint("access", "other", "project-a", "ap-southeast-1"))
	assert.NotEqual(t, base, OfficialAssetCredentialFingerprint("access", "secret", "project-b", "ap-southeast-1"))
	assert.NotEqual(t, base, OfficialAssetCredentialFingerprint("access", "secret", "project-a", "us-east-1"))
}

func TestOfficialAssetCredentialLifecycleIsAtomicAndFenced(t *testing.T) {
	truncateTables(t)
	channel := officialAssetTestChannel(t, 601)
	input := &dto.ChannelAssetCredentialInput{AccessKeyID: "access-a", SecretAccessKey: "secret-a"}
	require.NoError(t, InsertChannelWithAssetCredential(channel, input))

	stored, err := GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "access-a", stored.AccessKeyID)

	key, fingerprint, err := ResolveAssetChannelCredential(channel)
	require.NoError(t, err)
	assert.Equal(t, "access-a|secret-a", key)
	assert.Equal(t, OfficialAssetCredentialFingerprint("access-a", "secret-a", "project-a", "ap-southeast-1"), fingerprint)

	asset := Asset{
		UserID: 601, Name: "remote", AssetKind: AssetKindGeneral,
		MediaType: "image", Status: AssetStatusReady,
	}
	require.NoError(t, DB.Create(&asset).Error)
	require.NoError(t, DB.Create(&AssetBinding{
		AssetID: asset.ID, UserID: asset.UserID, ChannelID: channel.Id,
		CredentialFingerprint: fingerprint, UpstreamProfile: string(dto.AssetUpstreamProfileOfficial),
		Status: AssetBindingStatusActive,
	}).Error)

	err = UpdateChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "access-b", SecretAccessKey: "secret-b",
	})
	assert.ErrorIs(t, err, ErrChannelHasActiveAssetResources)
	assert.ErrorIs(t, DeleteChannelAssetCredential(channel.Id), ErrChannelHasActiveAssetResources)

	stored, err = GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "access-a", stored.AccessKeyID)
	assert.Equal(t, "secret-a", stored.SecretAccessKey)

	require.NoError(t, DB.Model(&AssetBinding{}).Where("asset_id = ?", asset.ID).Update("status", AssetBindingStatusDeleted).Error)
	require.NoError(t, UpdateChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "access-b", SecretAccessKey: "secret-b",
	}))

	stored, err = GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "access-b", stored.AccessKeyID)
	assert.Equal(t, "secret-b", stored.SecretAccessKey)

	assert.ErrorIs(t, DeleteChannelAssetCredential(channel.Id), ErrAssetCredentialProfileActive)
	settings := channel.GetOtherSettings()
	settings.AssetUpstreamProfile = dto.AssetUpstreamProfileNone
	channel.SetOtherSettings(settings)
	require.NoError(t, DB.Model(channel).Update("settings", channel.OtherSettings).Error)
	require.NoError(t, DeleteChannelAssetCredential(channel.Id))
	stored, err = GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, stored)

	require.NoError(t, UpdateChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "access-c", SecretAccessKey: "secret-c",
	}))
	require.NoError(t, channel.Delete())
	stored, err = GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, stored)
}

func TestOfficialAssetCredentialInsertRollsBackWithInvalidAbility(t *testing.T) {
	truncateTables(t)
	channel := officialAssetTestChannel(t, 602)
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(
		"test:fail_asset_credential_create",
		func(tx *gorm.DB) {
			if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "channel_asset_credentials" {
				tx.AddError(assert.AnError)
			}
		},
	))
	t.Cleanup(func() {
		require.NoError(t, DB.Callback().Create().Remove("test:fail_asset_credential_create"))
	})

	err := InsertChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "access", SecretAccessKey: "secret",
	})
	require.Error(t, err)

	var channelCount, abilityCount, credentialCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Count(&channelCount).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilityCount).Error)
	require.NoError(t, DB.Model(&ChannelAssetCredential{}).Where("channel_id = ?", channel.Id).Count(&credentialCount).Error)
	assert.Zero(t, channelCount)
	assert.Zero(t, abilityCount)
	assert.Zero(t, credentialCount)
}

func TestOfficialAssetCredentialMigrationGuard(t *testing.T) {
	t.Run("blocks legacy active fingerprint", func(t *testing.T) {
		truncateTables(t)
		channel := officialAssetTestChannel(t, 603)
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, DB.Create(&AssetBinding{
			AssetID: 1, UserID: 603, ChannelID: channel.Id,
			CredentialFingerprint: AssetCredentialFingerprint(channel.GetBaseURL(), "legacy-access|legacy-secret", string(dto.AssetUpstreamProfileOfficial), "project-a", "ap-southeast-1"),
			UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
			Status:                AssetBindingStatusActive,
		}).Error)

		err := validateOfficialAssetCredentialMigration()
		require.ErrorContains(t, err, "migration blocked")
	})

	t.Run("accepts current v2 fingerprint", func(t *testing.T) {
		truncateTables(t)
		channel := officialAssetTestChannel(t, 604)
		require.NoError(t, InsertChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
			AccessKeyID: "access", SecretAccessKey: "secret",
		}))
		_, fingerprint, err := ResolveAssetChannelCredential(channel)
		require.NoError(t, err)
		require.NoError(t, DB.Create(&AssetBinding{
			AssetID: 1, UserID: 604, ChannelID: channel.Id,
			CredentialFingerprint: fingerprint,
			UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
			Status:                AssetBindingStatusActive,
		}).Error)

		require.NoError(t, validateOfficialAssetCredentialMigration())
	})

	t.Run("prunes orphan findings for a deleted channel instead of bricking", func(t *testing.T) {
		truncateTables(t)
		// Channel 989 does not exist, mirroring a finding left behind after a
		// channel was hard-deleted. Such an orphan must not block startup.
		require.NoError(t, DB.Create(&AssetReconciliationFinding{
			ChannelID:             989,
			CredentialFingerprint: "legacy-fingerprint",
			UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
			Status:                AssetReconciliationFindingOpen,
			FindingType:           AssetReconciliationOrphanUpstream,
			ScopeHash:             "scope-989",
		}).Error)

		require.NoError(t, validateOfficialAssetCredentialMigration())

		var count int64
		require.NoError(t, DB.Model(&AssetReconciliationFinding{}).
			Where("channel_id = ?", 989).Count(&count).Error)
		assert.Zero(t, count, "orphan finding must be pruned")
	})
}
