package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkSharesHostedImageAndRejectsForeignOrDeletedReferences(t *testing.T) {
	withHostedAssetDB(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(t.Context(), store)
	owner := newFunCloudHostedMaterialAdapter(7, "funcloud-customer")
	created, err := owner.CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	ref := "asset://" + created.ResourceID
	for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolFunCloudModelArkV3, dto.VideoUpstreamProtocolSynlinkVideoV1} {
		c, info := hostedVideoContext(t, true, ref, "https://cdn.example/direct.png")
		c.Request = c.Request.WithContext(ctx)
		info.ChannelOtherSettings.VideoUpstreamProtocol = protocol
		require.Nil(t, ValidateFunCloudHostedVideoMedia(c, info))
		facts := GetFunCloudHostedMediaFacts(c)
		require.Len(t, facts, 1)
		assert.Equal(t, created.ResourceID, facts[ref].AssetID)
		info.UserId = 8
		require.NotNil(t, ValidateFunCloudHostedVideoMedia(c, info))
	}
	c, info := hostedVideoContext(t, true, ref)
	c.Request = c.Request.WithContext(ctx)
	info.ChannelOtherSettings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolSynlinkVideoV1
	store.failHead = true
	require.NotNil(t, ValidateFunCloudHostedVideoMedia(c, info))
	store.failHead = false
	require.Nil(t, ValidateFunCloudHostedVideoMedia(c, info))
	fact := GetFunCloudHostedMediaFacts(c)[ref]
	require.NoError(t, owner.DeleteAsset(ctx, created.ResourceID))
	_, err = SignFunCloudHostedAssetURL(c.Request.Context(), fact)
	require.NoError(t, err)
	next, nextInfo := hostedVideoContext(t, true, ref)
	next.Request = next.Request.WithContext(ctx)
	nextInfo.ChannelOtherSettings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolSynlinkVideoV1
	require.NotNil(t, ValidateFunCloudHostedVideoMedia(next, nextInfo))
	task := &model.Task{}
	StageFunCloudHostedMediaSnapshot(c, task)
	require.Len(t, task.PrivateData.HostedMedia, 1)
}

func TestSynlinkOpaqueRejectionDoesNotChangeFunCloudPassthrough(t *testing.T) {
	for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolFunCloudModelArkV3, dto.VideoUpstreamProtocolSynlinkVideoV1} {
		c, info := hostedVideoContext(t, true, "asset://provider-opaque")
		info.ChannelOtherSettings.VideoUpstreamProtocol = protocol
		err := ValidateFunCloudHostedVideoMedia(c, info)
		if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			require.NotNil(t, err)
		} else {
			require.Nil(t, err)
		}
		assert.Empty(t, GetFunCloudHostedMediaFacts(c))
	}
}
