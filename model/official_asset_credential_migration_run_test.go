package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func legacyOfficialAssetMigrationInput(channelID int) OfficialAssetCredentialMigrationInput {
	return OfficialAssetCredentialMigrationInput{
		ChannelID:                      channelID,
		ModelAPIKey:                    "new-video-api-key",
		AssetAccessKeyID:               "legacy-access",
		AssetSecretAccessKey:           "legacy-secret",
		ExpectedProviderProject:        "project-a",
		ExpectedRegion:                 "ap-southeast-1",
		AcknowledgeSameProviderAccount: true,
	}
}

func TestRunOfficialAssetCredentialMigrationUpdatesAllIdentityRecords(t *testing.T) {
	truncateTables(t)
	channel := officialAssetTestChannel(t, 611)
	channel.Key = "legacy-access|legacy-secret"
	require.NoError(t, DB.Create(channel).Error)

	oldFingerprint := AssetCredentialFingerprint(
		channel.GetBaseURL(),
		channel.Key,
		string(dto.AssetUpstreamProfileOfficial),
		"project-a",
		"ap-southeast-1",
	)
	asset := Asset{UserID: 611, Name: "asset", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady}
	require.NoError(t, DB.Create(&asset).Error)
	binding := AssetBinding{
		AssetID: asset.ID, UserID: 611, ChannelID: channel.Id,
		CredentialFingerprint: oldFingerprint,
		UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
		ProviderProject:       "project-a",
		Region:                "ap-southeast-1",
		Status:                AssetBindingStatusActive,
	}
	require.NoError(t, DB.Create(&binding).Error)
	group := AssetGroupBinding{
		UserID: 611, ScopeKey: "usr:611", ChannelID: channel.Id,
		CredentialFingerprint: oldFingerprint,
		UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
		ProviderProject:       "project-a",
		Region:                "ap-southeast-1",
		GroupKind:             "general_aigc",
		Status:                AssetBindingStatusActive,
	}
	require.NoError(t, DB.Create(&group).Error)
	authorization := RealPersonAuthorization{
		UserID: 611, ChannelID: channel.Id, CredentialFingerprint: oldFingerprint,
		UpstreamProfile: string(dto.AssetUpstreamProfileOfficial),
		ProviderProject: "project-a", Region: "ap-southeast-1",
		Status: RealPersonAuthorizationAuthorized, ConsentTokenHash: "consent-611",
	}
	require.NoError(t, DB.Create(&authorization).Error)
	assetClaim := AssetOwnershipClaim{
		ProviderAccountFingerprint: oldFingerprint,
		UpstreamProfile:            string(dto.AssetUpstreamProfileOfficial),
		ProviderProject:            "project-a",
		Region:                     "ap-southeast-1",
		UpstreamResourceID:         "asset-upstream-611",
		AssetBindingID:             binding.ID,
		UserID:                     611,
	}
	require.NoError(t, DB.Create(&assetClaim).Error)
	groupClaim := AssetGroupOwnershipClaim{
		ProviderAccountFingerprint: oldFingerprint,
		UpstreamProfile:            string(dto.AssetUpstreamProfileOfficial),
		ProviderProject:            "project-a",
		Region:                     "ap-southeast-1",
		UpstreamResourceID:         "group-upstream-611",
		AssetGroupBindingID:        group.ID,
		UserID:                     611,
	}
	require.NoError(t, DB.Create(&groupClaim).Error)
	finding := NewAssetReconciliationFinding(
		channel.Id,
		oldFingerprint,
		string(dto.AssetUpstreamProfileOfficial),
		"project-a",
		"ap-southeast-1",
		"asset",
		"asset-upstream-611",
		AssetReconciliationMissingUpstream,
	)
	require.NoError(t, DB.Create(&finding).Error)

	input := legacyOfficialAssetMigrationInput(channel.Id)
	plan, err := RunOfficialAssetCredentialMigration(input, false)
	require.NoError(t, err)
	assert.False(t, plan.Applied)
	assert.Equal(t, 6, plan.LegacyRecords)
	credential, err := GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, credential)

	result, err := RunOfficialAssetCredentialMigration(input, true)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, 6, result.LegacyRecords)

	var updated Channel
	require.NoError(t, DB.First(&updated, channel.Id).Error)
	assert.Equal(t, "new-video-api-key", updated.Key)
	credential, err = GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, credential)
	assert.Equal(t, "legacy-access", credential.AccessKeyID)
	assert.Equal(t, "legacy-secret", credential.SecretAccessKey)

	newFingerprint := OfficialAssetCredentialFingerprint(
		"legacy-access",
		"legacy-secret",
		"project-a",
		"ap-southeast-1",
	)
	var updatedBinding AssetBinding
	var updatedGroup AssetGroupBinding
	var updatedAuthorization RealPersonAuthorization
	var updatedAssetClaim AssetOwnershipClaim
	var updatedGroupClaim AssetGroupOwnershipClaim
	var updatedFinding AssetReconciliationFinding
	require.NoError(t, DB.First(&updatedBinding, binding.ID).Error)
	require.NoError(t, DB.First(&updatedGroup, group.ID).Error)
	require.NoError(t, DB.First(&updatedAuthorization, authorization.ID).Error)
	require.NoError(t, DB.First(&updatedAssetClaim, assetClaim.ID).Error)
	require.NoError(t, DB.First(&updatedGroupClaim, groupClaim.ID).Error)
	require.NoError(t, DB.First(&updatedFinding, finding.ID).Error)
	assert.Equal(t, newFingerprint, updatedBinding.CredentialFingerprint)
	assert.Equal(t, newFingerprint, updatedGroup.CredentialFingerprint)
	assert.Equal(t, newFingerprint, updatedAuthorization.CredentialFingerprint)
	assert.Equal(t, newFingerprint, updatedAssetClaim.ProviderAccountFingerprint)
	assert.Equal(t, newFingerprint, updatedGroupClaim.ProviderAccountFingerprint)
	assert.Equal(t, newFingerprint, updatedFinding.CredentialFingerprint)
	expectedFinding := NewAssetReconciliationFinding(
		channel.Id,
		newFingerprint,
		finding.UpstreamProfile,
		finding.ProviderProject,
		finding.Region,
		finding.ResourceKind,
		finding.UpstreamResourceID,
		finding.FindingType,
	)
	assert.Equal(t, expectedFinding.ScopeHash, updatedFinding.ScopeHash)
	require.NoError(t, ValidateOfficialAssetCredentialMigration())
}

func TestRunOfficialAssetCredentialMigrationRollsBackOnOwnershipConflict(t *testing.T) {
	truncateTables(t)
	channel := officialAssetTestChannel(t, 612)
	channel.Key = "legacy-access|legacy-secret"
	require.NoError(t, DB.Create(channel).Error)
	oldFingerprint := AssetCredentialFingerprint(
		channel.GetBaseURL(),
		channel.Key,
		string(dto.AssetUpstreamProfileOfficial),
		"project-a",
		"ap-southeast-1",
	)
	newFingerprint := OfficialAssetCredentialFingerprint(
		"legacy-access",
		"legacy-secret",
		"project-a",
		"ap-southeast-1",
	)
	bindings := []AssetBinding{
		{
			AssetID: 1, UserID: 612, ChannelID: channel.Id,
			CredentialFingerprint: oldFingerprint,
			UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
			Status:                AssetBindingStatusActive,
		},
		{
			AssetID: 2, UserID: 612, ChannelID: channel.Id,
			CredentialFingerprint: newFingerprint,
			UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
			Status:                AssetBindingStatusActive,
		},
	}
	require.NoError(t, DB.Create(&bindings).Error)
	claims := []AssetOwnershipClaim{
		{
			ProviderAccountFingerprint: oldFingerprint,
			UpstreamProfile:            string(dto.AssetUpstreamProfileOfficial),
			ProviderProject:            "project-a",
			Region:                     "ap-southeast-1",
			UpstreamResourceID:         "same-resource",
			AssetBindingID:             bindings[0].ID,
			UserID:                     612,
		},
		{
			ProviderAccountFingerprint: newFingerprint,
			UpstreamProfile:            string(dto.AssetUpstreamProfileOfficial),
			ProviderProject:            "project-a",
			Region:                     "ap-southeast-1",
			UpstreamResourceID:         "same-resource",
			AssetBindingID:             bindings[1].ID,
			UserID:                     612,
		},
	}
	require.NoError(t, DB.Create(&claims).Error)

	_, err := RunOfficialAssetCredentialMigration(legacyOfficialAssetMigrationInput(channel.Id), true)
	require.Error(t, err)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, "legacy-access|legacy-secret", stored.Key)
	credential, credentialErr := GetChannelAssetCredential(channel.Id)
	require.NoError(t, credentialErr)
	assert.Nil(t, credential)
	var legacyClaim AssetOwnershipClaim
	require.NoError(t, DB.First(&legacyClaim, claims[0].ID).Error)
	assert.Equal(t, oldFingerprint, legacyClaim.ProviderAccountFingerprint)
}

func TestRunOfficialAssetCredentialMigrationRejectsUnverifiedIdentity(t *testing.T) {
	truncateTables(t)
	channel := officialAssetTestChannel(t, 613)
	channel.Key = "legacy-access|legacy-secret"
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&AssetBinding{
		AssetID:               1,
		UserID:                613,
		ChannelID:             channel.Id,
		CredentialFingerprint: "unexpected-fingerprint",
		UpstreamProfile:       string(dto.AssetUpstreamProfileOfficial),
		Status:                AssetBindingStatusActive,
	}).Error)

	input := legacyOfficialAssetMigrationInput(channel.Id)
	input.AcknowledgeSameProviderAccount = false
	_, err := RunOfficialAssetCredentialMigration(input, true)
	require.ErrorContains(t, err, "acknowledgement")

	input.AcknowledgeSameProviderAccount = true
	_, err = RunOfficialAssetCredentialMigration(input, false)
	require.ErrorContains(t, err, "unexpected credential fingerprint")
	credential, credentialErr := GetChannelAssetCredential(channel.Id)
	require.NoError(t, credentialErr)
	assert.Nil(t, credential)
}

func TestRunOfficialAssetCredentialMigrationRejectsCombinedModelCredential(t *testing.T) {
	input := legacyOfficialAssetMigrationInput(1)
	input.ModelAPIKey = "video-key|unexpected"

	_, err := RunOfficialAssetCredentialMigration(input, false)

	require.ErrorContains(t, err, "model_api_key must not contain")
}
