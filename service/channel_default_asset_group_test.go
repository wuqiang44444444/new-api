package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelDefaultAssetGroupNotRequired(t *testing.T) {
	t.Run("none protocol needs no declaration", func(t *testing.T) {
		channel := &model.Channel{Id: 72, Type: constant.ChannelTypeSeedanceLink}
		channel.SetOtherSettings(dto.ChannelOtherSettings{AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})

		status, err := GetChannelDefaultAssetGroupStatus(channel)
		require.NoError(t, err)
		assert.False(t, status.Supported)
		assert.False(t, status.Configured)

		_, err = CreateOrReuseChannelDefaultAssetGroup(context.Background(), channel)
		assert.True(t, errors.Is(err, ErrUnsupportedAssetOperation) || errors.Is(err, ErrAssetLibraryUnsupported))
	})

	t.Run("hosted follows the published declaration", func(t *testing.T) {
		withAssetGroupPolicyDB(t)
		pinPublishedSeedanceServiceArtifact(t)
		channel := &model.Channel{Id: 72, Type: constant.ChannelTypeSeedanceLink}
		channel.SetOtherSettings(dto.ChannelOtherSettings{
			VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3,
			AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted,
		})

		status, err := GetChannelDefaultAssetGroupStatus(channel)
		require.NoError(t, err)
		assert.False(t, status.Supported)
		assert.False(t, status.Configured)

		_, err = CreateOrReuseChannelDefaultAssetGroup(context.Background(), channel)
		require.ErrorIs(t, err, ErrUnsupportedAssetOperation)
	})

	t.Run("hosted without declaration fails closed", func(t *testing.T) {
		previous := seedanceplugin.Default
		pluginstore := seedanceplugin.NewStore()
		seedanceplugin.Default = pluginstore
		t.Cleanup(func() { seedanceplugin.Default = previous })
		channel := &model.Channel{Id: 72, Type: constant.ChannelTypeSeedanceLink}
		channel.SetOtherSettings(dto.ChannelOtherSettings{
			VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3,
			AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted,
		})

		_, err := GetChannelDefaultAssetGroupStatus(channel)
		require.ErrorIs(t, err, ErrAssetUpstreamUnavailable,
			"a missing declaration must fail, not masquerade as not-required")
		_, err = CreateOrReuseChannelDefaultAssetGroup(context.Background(), channel)
		require.ErrorIs(t, err, ErrAssetUpstreamUnavailable)
	})
}

type defaultGroupAdapterFake struct {
	created int
	result  assetadapter.GroupResult
	err     error
}

func (f *defaultGroupAdapterFake) CreateGroup(context.Context, assetadapter.GroupRequest) (assetadapter.GroupResult, error) {
	f.created++
	return f.result, f.err
}

func (*defaultGroupAdapterFake) GetGroup(context.Context, string) (assetadapter.GroupResult, error) {
	return assetadapter.GroupResult{}, nil
}

type searchableDefaultGroupAdapterFake struct {
	defaultGroupAdapterFake
	pages map[int][]assetadapter.GroupResult
	total int
	err   error
}

func (f *searchableDefaultGroupAdapterFake) ListGroups(_ context.Context, req assetadapter.GroupListRequest) ([]assetadapter.GroupResult, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.pages[req.Page], f.total, nil
}

func TestCreateOrReuseDefaultAssetGroupReusesFirstExactName(t *testing.T) {
	adapter := &searchableDefaultGroupAdapterFake{
		defaultGroupAdapterFake: defaultGroupAdapterFake{result: assetadapter.GroupResult{ResourceID: "new-group"}},
		pages: map[int][]assetadapter.GroupResult{
			1: {
				{ResourceID: "fuzzy", Name: "aigctokenaigeneral-copy"},
				{ResourceID: "first-exact", Name: DefaultAssetGroupName},
				{ResourceID: "second-exact", Name: DefaultAssetGroupName},
			},
		},
		total: 3,
	}

	groupID, action, err := createOrReuseDefaultAssetGroup(context.Background(), nil, adapter)

	require.NoError(t, err)
	assert.Equal(t, "first-exact", groupID)
	assert.Equal(t, DefaultAssetGroupActionReused, action)
	assert.Zero(t, adapter.created)
}

func TestCreateOrReuseDefaultAssetGroupExhaustsPagesBeforeCreating(t *testing.T) {
	firstPage := make([]assetadapter.GroupResult, defaultAssetGroupPageSize)
	for index := range firstPage {
		firstPage[index] = assetadapter.GroupResult{
			ResourceID: fmt.Sprintf("group-%d", index),
			Name:       "not-the-default",
		}
	}
	adapter := &searchableDefaultGroupAdapterFake{
		defaultGroupAdapterFake: defaultGroupAdapterFake{result: assetadapter.GroupResult{ResourceID: "new-group"}},
		pages: map[int][]assetadapter.GroupResult{
			1: firstPage,
			2: {{ResourceID: "page-two-exact", Name: DefaultAssetGroupName}},
		},
		total: defaultAssetGroupPageSize + 1,
	}

	groupID, action, err := createOrReuseDefaultAssetGroup(context.Background(), nil, adapter)

	require.NoError(t, err)
	assert.Equal(t, "page-two-exact", groupID)
	assert.Equal(t, DefaultAssetGroupActionReused, action)
	assert.Zero(t, adapter.created)
}

func TestCreateOrReuseDefaultAssetGroupStopsWhenSearchFails(t *testing.T) {
	adapter := &searchableDefaultGroupAdapterFake{
		defaultGroupAdapterFake: defaultGroupAdapterFake{result: assetadapter.GroupResult{ResourceID: "new-group"}},
		err:                     errors.New("query failed"),
	}

	_, _, err := createOrReuseDefaultAssetGroup(context.Background(), nil, adapter)

	require.ErrorIs(t, err, ErrAssetUpstreamError)
	assert.Zero(t, adapter.created)
}

func TestCreateOrReuseDefaultAssetGroupDoesNotCreateFromNamelessSearchResults(t *testing.T) {
	adapter := &searchableDefaultGroupAdapterFake{
		defaultGroupAdapterFake: defaultGroupAdapterFake{result: assetadapter.GroupResult{ResourceID: "new-group"}},
		pages: map[int][]assetadapter.GroupResult{
			1: {{ResourceID: "unknown-group"}},
		},
		total: 1,
	}

	_, _, err := createOrReuseDefaultAssetGroup(context.Background(), nil, adapter)

	require.ErrorIs(t, err, ErrAssetUpstreamError)
	assert.Zero(t, adapter.created)
}

func TestCreateOrReuseDefaultAssetGroupCreatesWhenSearchIsUnavailable(t *testing.T) {
	adapter := &defaultGroupAdapterFake{result: assetadapter.GroupResult{ResourceID: "created-group"}}

	groupID, action, err := createOrReuseDefaultAssetGroup(context.Background(), nil, adapter)

	require.NoError(t, err)
	assert.Equal(t, "created-group", groupID)
	assert.Equal(t, DefaultAssetGroupActionCreated, action)
	assert.Equal(t, 1, adapter.created)
}
