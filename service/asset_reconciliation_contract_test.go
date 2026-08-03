package service

import (
	"context"
	"testing"

	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reconciliationContractAdapter struct {
	assetRequests []assetadapter.AssetListRequest
	groupRequests []assetadapter.GroupListRequest
}

func (a *reconciliationContractAdapter) ListAssets(_ context.Context, request assetadapter.AssetListRequest) ([]assetadapter.AssetResult, int, error) {
	a.assetRequests = append(a.assetRequests, request)
	return []assetadapter.AssetResult{{ResourceID: "asset-" + request.GroupType}}, 1, nil
}

func (a *reconciliationContractAdapter) ListGroups(_ context.Context, request assetadapter.GroupListRequest) ([]assetadapter.GroupResult, int, error) {
	a.groupRequests = append(a.groupRequests, request)
	return []assetadapter.GroupResult{{ResourceID: "group-" + request.GroupType}}, 1, nil
}

func TestOfficialAssetReconciliationListsAndMergesAllRequiredGroupTypes(t *testing.T) {
	adapter := &reconciliationContractAdapter{}

	assets, err := listAllUpstreamAssets(context.Background(), adapter)
	require.NoError(t, err)
	groups, err := listAllUpstreamGroups(context.Background(), adapter)
	require.NoError(t, err)

	assert.Equal(t, map[string]struct{}{
		"asset-AIGC":         {},
		"asset-LivenessFace": {},
	}, assets)
	assert.Equal(t, map[string]struct{}{
		"group-AIGC":         {},
		"group-LivenessFace": {},
	}, groups)
	require.Len(t, adapter.assetRequests, 2)
	require.Len(t, adapter.groupRequests, 2)
	assert.Equal(t, "AIGC", adapter.assetRequests[0].GroupType)
	assert.Equal(t, "LivenessFace", adapter.assetRequests[1].GroupType)
	assert.Equal(t, 100, adapter.assetRequests[0].PageSize)
	assert.Equal(t, "AIGC", adapter.groupRequests[0].GroupType)
	assert.Equal(t, "LivenessFace", adapter.groupRequests[1].GroupType)
}
