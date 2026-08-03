package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelWithAllowedIDsSelectsPriorityWithinAllowedSet(t *testing.T) {
	truncateTables(t)
	highPriority := int64(100)
	lowPriority := int64(10)
	weight := uint(1)
	channels := []Channel{
		{Id: 301, Name: "unbound-high", Key: "key-high", Status: common.ChannelStatusEnabled},
		{Id: 302, Name: "bound-low", Key: "key-low", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "default", Model: "video-model", ChannelId: 301, Enabled: true, Priority: &highPriority, Weight: weight},
		{Group: "default", Model: "video-model", ChannelId: 302, Enabled: true, Priority: &lowPriority, Weight: weight},
	}).Error)

	channel, err := GetChannelWithAllowedIDs("default", "video-model", 0, "/v1/video/generations", map[int]struct{}{302: {}})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 302, channel.Id)
}

func TestChannelHasActiveAssetResources(t *testing.T) {
	truncateTables(t)
	binding := AssetBinding{AssetID: 1, UserID: 101, ChannelID: 77, CredentialFingerprint: "fingerprint", Status: AssetBindingStatusActive}
	require.NoError(t, DB.Create(&binding).Error)

	hasActive, err := ChannelHasActiveAssetResources(77)
	require.NoError(t, err)
	assert.True(t, hasActive)

	hasActive, err = ChannelHasActiveAssetResources(88)
	require.NoError(t, err)
	assert.False(t, hasActive)

	require.NoError(t, DB.Model(&binding).Update("status", AssetBindingStatusDeleted).Error)
	group := AssetGroupBinding{UserID: 101, ScopeKey: "usr:101", ChannelID: 88, CredentialFingerprint: "group-fingerprint", GroupKind: "general_aigc", Status: AssetBindingStatusActive}
	require.NoError(t, DB.Create(&group).Error)
	hasActive, err = ChannelHasActiveAssetResources(88)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestAssetCredentialFingerprintBindsEndpointKeyAndProfile(t *testing.T) {
	base := AssetCredentialFingerprint("https://upstream.example/", "key", "ark_assets")
	assert.Equal(t, base, AssetCredentialFingerprint("https://upstream.example", "key", "ark_assets"))
	assert.NotEqual(t, base, AssetCredentialFingerprint("https://other.example", "key", "ark_assets"))
	assert.NotEqual(t, base, AssetCredentialFingerprint("https://upstream.example", "other-key", "ark_assets"))
	assert.NotEqual(t, base, AssetCredentialFingerprint("https://upstream.example", "key", "relay_assets"))
}

func TestAssetOperationLeaseCanBeReclaimedAndOnlyOwnerCanFinish(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	job := AssetOperationJob{IdempotencyKey: "expired-lease", Kind: "poll_binding", Status: AssetJobRunning, LockedBy: "stopped-runner", LockedUntil: now - 1}
	require.NoError(t, DB.Create(&job).Error)

	claimed, err := ClaimNextAssetOperationJob("replacement-runner", 60, []string{"poll_binding"})
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, job.ID, claimed.ID)
	assert.Equal(t, "replacement-runner", claimed.LockedBy)
	assert.Error(t, FinishAssetOperationJob(job.ID, "stopped-runner"))
	require.NoError(t, FinishAssetOperationJob(job.ID, "replacement-runner"))
}

func TestEnsureAssetOperationJobRevivesDeadUserRetry(t *testing.T) {
	truncateTables(t)
	job := AssetOperationJob{IdempotencyKey: "delete-binding:42", Kind: "delete_binding", Status: AssetJobDead, AttemptCount: 12, MaxAttempts: 12, LastError: "failed"}
	require.NoError(t, DB.Create(&job).Error)

	revived, err := EnsureAssetOperationJob(DB, &AssetOperationJob{IdempotencyKey: job.IdempotencyKey, Kind: job.Kind}, true)
	require.NoError(t, err)
	assert.Equal(t, AssetJobPending, revived.Status)
	assert.Zero(t, revived.AttemptCount)
	assert.Empty(t, revived.LastError)
}

func TestAssetOperationClaimCanRestrictDisabledModeToCleanupJobs(t *testing.T) {
	truncateTables(t)
	jobs := []AssetOperationJob{
		{IdempotencyKey: "update-binding:1", Kind: "update_binding", Status: AssetJobPending},
		{IdempotencyKey: "delete-binding:1", Kind: "delete_binding", Status: AssetJobPending},
	}
	require.NoError(t, DB.Create(&jobs).Error)

	claimed, err := ClaimNextAssetOperationJob("cleanup-runner", 60, []string{"delete_binding", "delete_group"})
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, "delete_binding", claimed.Kind)
}

func TestRemoteAssetCreateIdempotencyReplaysWithoutDuplicateAsset(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 404, Username: "remote-idempotency"}).Error)
	channel, fingerprint := seedAssetLifecycleChannel(t, 9, "credential", "relay_assets")
	newRecords := func(requestHMAC string) (*Asset, *AssetBinding, *AssetCreateIdempotency) {
		asset := &Asset{UserID: 404, Name: "remote", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusCreating}
		binding := newAssetBindingForTest(t, channel, 404, fingerprint, "relay_assets")
		idempotency := &AssetCreateIdempotency{UserID: 404, Endpoint: "/v1/assets", KeyHash: "key-hash", RequestHMAC: requestHMAC, Status: AssetCreateIdempotencyCreating, ExpiresAt: common.GetTimestamp() + 3600}
		return asset, binding, idempotency
	}

	asset, binding, idempotency := newRecords("request-a")
	created, _, replay, err := CreateRemoteAssetWithQuota(asset, "https://example.com/a.png", common.GetTimestamp()+600, binding, idempotency, 10, 300)
	require.NoError(t, err)
	assert.False(t, replay)
	var watchdog AssetOperationJob
	require.NoError(t, DB.First(&watchdog, "idempotency_key = ?", fmt.Sprintf("resolve-unknown-create:%d", binding.ID)).Error)
	assert.Equal(t, AssetJobPending, watchdog.Status)
	assert.GreaterOrEqual(t, watchdog.NextAttemptAt, common.GetTimestamp()+299)

	asset, binding, idempotency = newRecords("request-a")
	replayed, _, replay, err := CreateRemoteAssetWithQuota(asset, "https://example.com/a.png", common.GetTimestamp()+600, binding, idempotency, 10, 300)
	require.NoError(t, err)
	assert.True(t, replay)
	assert.Equal(t, created.ID, replayed.ID)

	asset, binding, idempotency = newRecords("request-b")
	_, _, _, err = CreateRemoteAssetWithQuota(asset, "https://example.com/a.png", common.GetTimestamp()+600, binding, idempotency, 10, 300)
	assert.ErrorIs(t, err, ErrAssetIdempotencyConflict)
}

func TestRemoteAssetCreateRechecksRealPersonAuthorizationInsideTransaction(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 405, Username: "revoked-authorization"}).Error)
	channel, fingerprint := seedAssetLifecycleChannel(t, 9, "credential", "ark_assets")
	authorization := RealPersonAuthorization{
		UserID: 405, Status: RealPersonAuthorizationRevoked, RevokedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&authorization).Error)

	asset := &Asset{
		UserID: 405, Name: "real person", AssetKind: AssetKindRealPerson,
		MediaType: "image", Status: AssetStatusCreating, AuthorizationID: &authorization.ID,
	}
	binding := newAssetBindingForTest(t, channel, 405, fingerprint, "ark_assets")

	_, _, _, err := CreateRemoteAssetWithQuota(asset, "https://example.com/a.png", common.GetTimestamp()+600, binding, nil, 10, 300)
	assert.ErrorIs(t, err, ErrAssetAuthorizationNotAuthorized)
	var assetCount, bindingCount int64
	require.NoError(t, DB.Model(&Asset{}).Count(&assetCount).Error)
	require.NoError(t, DB.Model(&AssetBinding{}).Count(&bindingCount).Error)
	assert.Zero(t, assetCount)
	assert.Zero(t, bindingCount)
}
