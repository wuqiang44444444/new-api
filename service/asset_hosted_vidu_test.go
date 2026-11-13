package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduChannelsShareHostedAssetLifecycle(t *testing.T) {
	withHostedAssetDB(t)
	pinPublishedSeedanceServiceArtifact(t)
	store := newFakeHostedImageStore()
	ctx := withHostedImageSession(t.Context(), store)
	var adapters []assetadapter.Adapter
	for _, variant := range []struct {
		model   string
		mapping string
	}{
		{"customer-cn", `{"customer-cn":"viduq3-drama-mini"}`},
		{"customer-global", `{"customer-global":"viduq3-drama-ab-mini"}`},
	} {
		channel := &model.Channel{Type: constant.ChannelTypeSeedanceLink}
		channel.ModelMapping = common.GetPointer(variant.mapping)
		channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted})
		adapter, err := seedanceAssetAdapter(channel, 7, variant.model)
		require.NoError(t, err)
		adapters = append(adapters, adapter)
		assert.True(t, adapter.Supports("general", "image"))
		assert.True(t, adapter.Supports("general", "audio"))
		assert.True(t, adapter.Supports("general", "video"))
	}
	created, err := adapters[0].CreateAsset(ctx, hostedImageCreateRequest(t, ""))
	require.NoError(t, err)
	fetched, err := adapters[1].GetAsset(ctx, created.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "ready", fetched.Status)
	ref := "asset://" + created.ResourceID
	c, info := hostedVideoContext(t, true, ref, "https://cdn.example/direct.png")
	c.Request = c.Request.WithContext(ctx)
	info.ChannelOtherSettings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolViduModelArkV3
	require.Nil(t, ValidateFunCloudHostedVideoMedia(c, info))
	fact := GetFunCloudHostedMediaFacts(c)[ref]
	assert.Equal(t, created.ResourceID, fact.AssetID)
	require.NoError(t, adapters[1].DeleteAsset(ctx, created.ResourceID))
	_, err = adapters[0].GetAsset(ctx, created.ResourceID)
	assert.ErrorIs(t, err, assetadapter.ErrAssetResourceNotFound)
	_, err = SignFunCloudHostedAssetURL(ctx, fact)
	require.NoError(t, err, "accepted facts survive deletion")
	require.Len(t, store.puts, 1, "deletion never removes the object")
}
