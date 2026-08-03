package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialAssetReconciliationReportsDifferencesWithoutClaimingOrphans(t *testing.T) {
	truncate(t)
	const (
		channelID   = 77
		fingerprint = "provider-account"
		profile     = "official_action_assets"
		project     = "project-a"
		region      = "ap-southeast-1"
	)
	bindings := []model.AssetBinding{
		{AssetID: 1, UserID: 1, ChannelID: channelID, CredentialFingerprint: fingerprint, UpstreamProfile: profile, ProviderProject: project, Region: region, UpstreamResourceID: "asset-bound", Status: model.AssetBindingStatusActive},
		{AssetID: 2, UserID: 1, ChannelID: channelID, CredentialFingerprint: fingerprint, UpstreamProfile: profile, ProviderProject: project, Region: region, UpstreamResourceID: "asset-missing", Status: model.AssetBindingStatusActive},
	}
	for i := range bindings {
		require.NoError(t, model.DB.Create(&bindings[i]).Error)
	}
	groups := []model.AssetGroupBinding{
		{UserID: 1, ScopeKey: "scope-a", ChannelID: channelID, CredentialFingerprint: fingerprint, UpstreamProfile: profile, ProviderProject: project, Region: region, GroupKind: "general", UpstreamResourceID: "group-bound", Status: model.AssetBindingStatusActive},
		{UserID: 1, ScopeKey: "scope-b", ChannelID: channelID, CredentialFingerprint: fingerprint, UpstreamProfile: profile, ProviderProject: project, Region: region, GroupKind: "real_person", UpstreamResourceID: "group-missing", Status: model.AssetBindingStatusActive},
	}
	for i := range groups {
		require.NoError(t, model.DB.Create(&groups[i]).Error)
	}

	findings, orphanCount, missingCount, err := compareOfficialAssetInventory(
		channelID,
		fingerprint,
		profile,
		project,
		region,
		map[string]struct{}{"asset-bound": {}, "asset-orphan": {}},
		map[string]struct{}{"group-bound": {}, "group-orphan": {}},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, orphanCount)
	assert.Equal(t, 2, missingCount)
	require.Len(t, findings, 4)

	require.NoError(t, model.SaveAssetReconciliationFindings(model.DB, channelID, fingerprint, findings))
	var openCount int64
	require.NoError(t, model.DB.Model(&model.AssetReconciliationFinding{}).
		Where("channel_id = ? AND status = ?", channelID, model.AssetReconciliationFindingOpen).
		Count(&openCount).Error)
	assert.Equal(t, int64(4), openCount)
	var claims int64
	require.NoError(t, model.DB.Model(&model.AssetOwnershipClaim{}).Count(&claims).Error)
	assert.Zero(t, claims)

	require.NoError(t, model.SaveAssetReconciliationFindings(model.DB, channelID, fingerprint, nil))
	var resolvedCount int64
	require.NoError(t, model.DB.Model(&model.AssetReconciliationFinding{}).
		Where("channel_id = ? AND status = ?", channelID, model.AssetReconciliationFindingResolved).
		Count(&resolvedCount).Error)
	assert.Equal(t, int64(4), resolvedCount)
}

func TestAssetMigrationRevalidatesSourceAndRealPersonAuthorization(t *testing.T) {
	truncate(t)
	readyRealPerson := model.Asset{
		UserID: 991, AppID: 1, Name: "portrait", AssetKind: model.AssetKindRealPerson,
		MediaType: "image", Status: model.AssetStatusReady,
	}
	require.NoError(t, model.DB.Create(&readyRealPerson).Error)

	_, err := MigrateRemoteAsset(
		context.Background(),
		readyRealPerson.UserID,
		1,
		"default",
		"default",
		"migration-key",
		readyRealPerson.PublicID,
		dto.MigrateAssetRequest{
			Model:           "seedance-model",
			Source:          dto.AssetSource{Type: "url", URL: "https://example.com/portrait.png"},
			MigrationReason: "move to a new provider scope",
		},
	)
	assert.True(t, errors.Is(err, ErrRealPersonAuthorizationNotReady))

	notReady := model.Asset{
		UserID: 992, AppID: 1, Name: "general", AssetKind: model.AssetKindGeneral,
		MediaType: "image", Status: model.AssetStatusProcessing,
	}
	require.NoError(t, model.DB.Create(&notReady).Error)
	_, err = MigrateRemoteAsset(
		context.Background(),
		notReady.UserID,
		1,
		"default",
		"default",
		"migration-key",
		notReady.PublicID,
		dto.MigrateAssetRequest{
			Model:           "seedance-model",
			Source:          dto.AssetSource{Type: "url", URL: "https://example.com/general.png"},
			MigrationReason: "move to a new provider scope",
		},
	)
	assert.True(t, errors.Is(err, ErrAssetBindingRequired))
}
