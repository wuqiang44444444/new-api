package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAssetLifecycleChannel(t *testing.T, id int, key, profile string) (*Channel, string) {
	t.Helper()
	baseURL := fmt.Sprintf("https://asset-channel-%d.example", id)
	otherSettings := dto.ChannelOtherSettings{
		VideoUpstreamProfile:    dto.VideoUpstreamProfileThirdPartyRelay,
		VideoUpstreamCreatePath: "/v1/media/generations", VideoUpstreamQueryPathTemplate: "/v1/media/tasks/{task_id}",
		AssetUpstreamProfile: dto.AssetUpstreamProfile(profile),
		LinkImplementation:   dto.LinkImplementationRef{ID: LinkImplementationMoxingSeedanceMedia, Version: LinkImplementationVersionV1},
	}
	if profile == string(dto.AssetUpstreamProfileArk) {
		otherSettings.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartyReverseProxy
		otherSettings.VideoUpstreamCreatePath = ""
		otherSettings.VideoUpstreamQueryPathTemplate = ""
		otherSettings.LinkImplementation.ID = LinkImplementationMoxingSeedanceArk
	}
	settings, err := common.Marshal(otherSettings)
	require.NoError(t, err)
	channel := &Channel{
		Id: id, Type: constant.ChannelTypeDoubaoVideo, Key: key,
		Status: common.ChannelStatusEnabled, Name: fmt.Sprintf("asset-channel-%d", id),
		BaseURL: &baseURL, OtherSettings: string(settings), Models: VideoSKUSeedance20Oversea, Group: "default",
	}
	require.NoError(t, DB.Create(channel).Error)
	return channel, AssetCredentialFingerprint(baseURL, key, profile)
}

func newAssetBindingForTest(t *testing.T, channel *Channel, userID int, fingerprint, profile string) *AssetBinding {
	t.Helper()
	implementation, ok := ResolveChannelLinkImplementation(channel)
	require.True(t, ok)
	return &AssetBinding{
		UserID: userID, ChannelID: channel.Id, CredentialFingerprint: fingerprint,
		LinkImplementationID: implementation.ID, LinkImplementationVer: implementation.Version, LinkImplementationHash: implementation.ContentHash,
		UpstreamProfile: profile, RequestedModel: channel.Models, Status: AssetBindingStatusCreating,
	}
}

func TestRemoteAssetCreateRevalidatesChannelInsideTransaction(t *testing.T) {
	t.Run("disabled channel", func(t *testing.T) {
		truncateTables(t)
		require.NoError(t, DB.Create(&User{Id: 501, Username: "disabled-channel"}).Error)
		channel, fingerprint := seedAssetLifecycleChannel(t, 501, "key-before-disable", "relay_assets")
		require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)

		asset := &Asset{UserID: 501, Name: "remote", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusCreating}
		binding := newAssetBindingForTest(t, channel, 501, fingerprint, "relay_assets")
		_, _, _, err := CreateRemoteAssetWithQuota(asset, "https://example.com/a.png", common.GetTimestamp()+600, binding, nil, 10, 300)

		assert.ErrorIs(t, err, ErrAssetChannelUnavailable)
		assertNoAssetCreateRows(t)
	})

	t.Run("rotated credential", func(t *testing.T) {
		truncateTables(t)
		require.NoError(t, DB.Create(&User{Id: 502, Username: "rotated-channel"}).Error)
		channel, _ := seedAssetLifecycleChannel(t, 502, "key-after-rotation", "relay_assets")
		staleFingerprint := AssetCredentialFingerprint(channel.GetBaseURL(), "key-before-rotation", "relay_assets")

		asset := &Asset{UserID: 502, Name: "remote", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusCreating}
		binding := newAssetBindingForTest(t, channel, 502, staleFingerprint, "relay_assets")
		_, _, _, err := CreateRemoteAssetWithQuota(asset, "https://example.com/a.png", common.GetTimestamp()+600, binding, nil, 10, 300)

		assert.ErrorIs(t, err, ErrAssetChannelCredentialChanged)
		assertNoAssetCreateRows(t)
	})
}

func TestChannelCredentialMutationAndDeletionRespectAssetFence(t *testing.T) {
	truncateTables(t)
	channel, fingerprint := seedAssetLifecycleChannel(t, 503, "stable-key", "relay_assets")
	asset := Asset{UserID: 503, Name: "remote", AssetKind: AssetKindGeneral, MediaType: "image", Status: AssetStatusReady}
	require.NoError(t, DB.Create(&asset).Error)
	binding := *newAssetBindingForTest(t, channel, asset.UserID, fingerprint, "relay_assets")
	binding.AssetID = asset.ID
	binding.Status = AssetBindingStatusActive
	require.NoError(t, DB.Create(&binding).Error)

	updated := *channel
	updated.Key = "rotated-key"
	err := updated.Update()
	assert.ErrorIs(t, err, ErrChannelHasActiveAssetResources)
	var persisted Channel
	require.NoError(t, DB.First(&persisted, "id = ?", channel.Id).Error)
	assert.Equal(t, "stable-key", persisted.Key)

	err = channel.Delete()
	assert.ErrorIs(t, err, ErrChannelHasActiveAssetResources)
	require.NoError(t, DB.First(&persisted, "id = ?", channel.Id).Error)
}

func TestUserDeleteFinalizesOnlyAuthorizationsWithoutResources(t *testing.T) {
	truncateTables(t)
	user := User{Id: 504, Username: "asset-cleanup-user"}
	require.NoError(t, DB.Create(&user).Error)
	channel, fingerprint := seedAssetLifecycleChannel(t, 504, "cleanup-key", "ark_assets")

	emptyAuthorization := RealPersonAuthorization{UserID: user.Id, Status: RealPersonAuthorizationAuthorized}
	localAuthorization := RealPersonAuthorization{UserID: user.Id, Status: RealPersonAuthorizationAuthorized}
	remoteAuthorization := RealPersonAuthorization{UserID: user.Id, Status: RealPersonAuthorizationAuthorized}
	require.NoError(t, DB.Create(&[]*RealPersonAuthorization{&emptyAuthorization, &localAuthorization, &remoteAuthorization}).Error)

	localAsset := Asset{UserID: user.Id, Name: "local-only", AssetKind: AssetKindRealPerson, MediaType: "image", AuthorizationID: &localAuthorization.ID, Status: AssetStatusReady}
	remoteAsset := Asset{UserID: user.Id, Name: "remote", AssetKind: AssetKindRealPerson, MediaType: "image", AuthorizationID: &remoteAuthorization.ID, Status: AssetStatusReady}
	require.NoError(t, DB.Create(&[]*Asset{&localAsset, &remoteAsset}).Error)
	remoteBinding := *newAssetBindingForTest(t, channel, user.Id, fingerprint, "ark_assets")
	remoteBinding.AssetID = remoteAsset.ID
	remoteBinding.Status = AssetBindingStatusActive
	require.NoError(t, DB.Create(&remoteBinding).Error)

	require.NoError(t, user.Delete())

	require.NoError(t, DB.First(&emptyAuthorization, "id = ?", emptyAuthorization.ID).Error)
	require.NoError(t, DB.First(&localAuthorization, "id = ?", localAuthorization.ID).Error)
	require.NoError(t, DB.First(&remoteAuthorization, "id = ?", remoteAuthorization.ID).Error)
	assert.Equal(t, RealPersonAuthorizationDeleted, emptyAuthorization.Status)
	assert.Equal(t, RealPersonAuthorizationDeleted, localAuthorization.Status)
	assert.Equal(t, RealPersonAuthorizationRevoked, remoteAuthorization.Status)

	require.NoError(t, DB.First(&localAsset, "id = ?", localAsset.ID).Error)
	require.NoError(t, DB.First(&remoteAsset, "id = ?", remoteAsset.ID).Error)
	assert.Equal(t, AssetStatusDeleted, localAsset.Status)
	assert.NotZero(t, localAsset.DeletedAt)
	assert.Equal(t, AssetStatusDeleting, remoteAsset.Status)
	require.NoError(t, DB.First(&remoteBinding, "id = ?", remoteBinding.ID).Error)
	assert.Equal(t, AssetBindingStatusDeleting, remoteBinding.Status)
	var job AssetOperationJob
	require.NoError(t, DB.First(&job, "idempotency_key = ?", fmt.Sprintf("delete-binding:%d", remoteBinding.ID)).Error)
	assert.Equal(t, AssetJobPending, job.Status)
}

func assertNoAssetCreateRows(t *testing.T) {
	t.Helper()
	var assetCount, bindingCount int64
	require.NoError(t, DB.Model(&Asset{}).Count(&assetCount).Error)
	require.NoError(t, DB.Model(&AssetBinding{}).Count(&bindingCount).Error)
	assert.Zero(t, assetCount)
	assert.Zero(t, bindingCount)
	var jobCount int64
	require.NoError(t, DB.Model(&AssetOperationJob{}).Count(&jobCount).Error)
	assert.Zero(t, jobCount)
}
