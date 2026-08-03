package assets

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This read-only contract test is intentionally credential-gated. It validates
// the real Action signature and response envelope without creating resources.
func TestOfficialActionLiveListAssets(t *testing.T) {
	if os.Getenv("TEST_BYTEPLUS_ASSET_LIVE") != "1" {
		t.Skip("set TEST_BYTEPLUS_ASSET_LIVE=1 to run the live official Action contract")
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

	client := &http.Client{Timeout: 20 * time.Second}
	adapter, err := NewOfficialActionAdapter(baseURL, accessKey+"|"+secretKey, region, project, client)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, total, err := adapter.ListAssets(ctx, AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})

	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 0)
}
