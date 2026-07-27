package service

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
	"github.com/stretchr/testify/require"
)

func TestOfficialAssetLiveReconciliationListsAllGroupTypes(t *testing.T) {
	if os.Getenv("TEST_BYTEPLUS_ASSET_LIVE") != "1" {
		t.Skip("set TEST_BYTEPLUS_ASSET_LIVE=1 to run the live official Action reconciliation contract")
	}
	baseURL := os.Getenv("TEST_BYTEPLUS_ASSET_BASE_URL")
	accessKey := os.Getenv("TEST_BYTEPLUS_ACCESS_KEY")
	secretKey := os.Getenv("TEST_BYTEPLUS_SECRET_KEY")
	region := os.Getenv("TEST_BYTEPLUS_REGION")
	project := os.Getenv("TEST_BYTEPLUS_PROVIDER_PROJECT")
	require.NotEmpty(t, baseURL)
	require.NotEmpty(t, accessKey)
	require.NotEmpty(t, secretKey)
	require.NotEmpty(t, region)
	require.NotEmpty(t, project)

	adapter, err := assetadapter.NewOfficialActionAdapter(
		baseURL,
		accessKey+"|"+secretKey,
		region,
		project,
		&http.Client{Timeout: 20 * time.Second},
	)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	assets, err := listAllUpstreamAssets(ctx, adapter)
	require.NoError(t, err)
	require.NotNil(t, assets)
	groups, err := listAllUpstreamGroups(ctx, adapter)
	require.NoError(t, err)
	require.NotNil(t, groups)
}
