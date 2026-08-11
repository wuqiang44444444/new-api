// Package assets implements Seedance asset-library protocols.
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
func TestBytePlusActionLiveListAssets(t *testing.T) {
	if os.Getenv("TEST_BYTEPLUS_ASSET_LIVE") != "1" {
		t.Skip("set TEST_BYTEPLUS_ASSET_LIVE=1 to run the live official Action contract")
	}
	accessKey := os.Getenv("TEST_BYTEPLUS_ACCESS_KEY")
	secretKey := os.Getenv("TEST_BYTEPLUS_SECRET_KEY")
	region := os.Getenv("TEST_BYTEPLUS_REGION")
	project := os.Getenv("TEST_BYTEPLUS_PROVIDER_PROJECT")
	require.NotEmpty(t, accessKey)
	require.NotEmpty(t, secretKey)
	require.NotEmpty(t, region)
	require.NotEmpty(t, project)

	client := &http.Client{Timeout: 20 * time.Second}
	adapter, err := NewBytePlusActionAdapter(accessKey+"|"+secretKey, region, project, client)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, total, err := adapter.ListAssets(ctx, AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})

	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 0)
}

func TestVolcengineActionLiveListAssets(t *testing.T) {
	if os.Getenv("TEST_VOLCENGINE_ASSET_LIVE") != "1" {
		t.Skip("set TEST_VOLCENGINE_ASSET_LIVE=1 to run the live Volcengine Action contract")
	}
	accessKey := os.Getenv("TEST_VOLCENGINE_ACCESS_KEY")
	secretKey := os.Getenv("TEST_VOLCENGINE_SECRET_KEY")
	project := os.Getenv("TEST_VOLCENGINE_PROVIDER_PROJECT")
	require.NotEmpty(t, accessKey)
	require.NotEmpty(t, secretKey)
	require.NotEmpty(t, project)

	client := &http.Client{Timeout: 20 * time.Second}
	adapter, err := NewVolcengineActionAdapter(accessKey+"|"+secretKey, project, client)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, total, err := adapter.ListAssets(ctx, AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})

	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 0)
}
