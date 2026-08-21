package assets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCMCCAICCV2CreateAssetUsesSignedRESTContract(t *testing.T) {
	var captured *http.Request
	var requestBody map[string]any
	adapter, err := NewCMCCAICCV2Adapter("ACCESS|SECRET", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		captured = req.Clone(req.Context())
		body, readErr := io.ReadAll(req.Body)
		require.NoError(t, readErr)
		require.NoError(t, common.Unmarshal(body, &requestBody))
		return assetJSONResponse(`{"requestId":"request-1","state":"OK","errorCode":null,"errorMessage":null,"body":"asset-1"}`), nil
	}))
	require.NoError(t, err)
	adapter.now = func() time.Time { return time.Date(2026, time.August, 21, 1, 2, 3, 0, time.UTC) }

	result, err := adapter.CreateAsset(context.Background(), AssetRequest{
		GroupResourceID: "group-1", URL: "https://cdn.example.com/source.png", Name: "portrait", MediaType: "image",
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, http.MethodPost, captured.Method)
	assert.Equal(t, "/api/openapi-maas/exp/aicc/v2/asset", captured.URL.Path)
	assert.Equal(t, "ACCESS", captured.URL.Query().Get("AccessKey"))
	assert.Equal(t, "2026-08-21T01:02:03Z", captured.URL.Query().Get("Timestamp"))
	assert.Equal(t, "HmacSHA256", captured.URL.Query().Get("SignatureMethod"))
	assert.Equal(t, "V2.0", captured.URL.Query().Get("SignatureVersion"))
	assert.NotEmpty(t, captured.URL.Query().Get("SignatureNonce"))
	assert.Len(t, captured.URL.Query().Get("Signature"), 64)
	assert.Empty(t, captured.Header.Get("Authorization"))
	assert.NotContains(t, captured.URL.RawQuery, "SECRET")
	assert.Equal(t, "group-1", requestBody["groupId"])
	assert.Equal(t, "portrait", requestBody["assetName"])
	assert.Equal(t, "https://cdn.example.com/source.png", requestBody["assetUrl"])
	assert.Equal(t, "Image", requestBody["assetType"])
	assert.Equal(t, "asset-1", result.ResourceID)
	assert.Equal(t, "processing", result.Status)
}

func TestCMCCAICCV2ImplementsReadOnlyConnectivityCRUDAndVerification(t *testing.T) {
	requests := make([]string, 0, 9)
	adapter, err := NewCMCCAICCV2Adapter("ACCESS|SECRET", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		key := req.Method + " " + req.URL.Path
		requests = append(requests, key)
		switch key {
		case "POST /api/openapi-maas/exp/aicc/v2/asset/query":
			return assetJSONResponse(`{"requestId":"request-list","state":"OK","body":{"data":[],"total":0}}`), nil
		case "GET /api/openapi-maas/exp/aicc/v2/asset/asset-1":
			return assetJSONResponse(`{"requestId":"request-get","state":"OK","body":{"assetId":"asset-1","groupId":"group-1","assetName":"portrait","assetType":"Video","assetUrl":"https://signed.example/private?secret=value","status":"ACTIVE"}}`), nil
		case "PUT /api/openapi-maas/exp/aicc/v2/asset/asset-1":
			return assetJSONResponse(`{"requestId":"request-update","state":"OK","body":{"assetId":"asset-1","status":"ACTIVE"}}`), nil
		case "DELETE /api/openapi-maas/exp/aicc/v2/asset/asset-1":
			return assetJSONResponse(`{"requestId":"request-delete","state":"OK","body":true}`), nil
		case "POST /api/openapi-maas/exp/aicc/v2/asset-group/":
			return assetJSONResponse(`{"requestId":"request-group-create","state":"OK","body":{"groupId":"group-1","groupType":"AIGC","groupName":"characters"}}`), nil
		case "GET /api/openapi-maas/exp/aicc/v2/asset-group/group-1":
			return assetJSONResponse(`{"requestId":"request-group-get","state":"OK","body":{"groupId":"group-1","groupType":"AIGC","groupName":"characters"}}`), nil
		case "DELETE /api/openapi-maas/exp/aicc/v2/asset-group/group-1":
			return assetJSONResponse(`{"requestId":"request-group-delete","state":"OK","body":true}`), nil
		case "POST /api/openapi-maas/exp/aicc/v2/real-person-auth/sessions":
			return assetJSONResponse(`{"requestId":"request-session","state":"OK","body":{"bytedToken":"opaque-session","h5Link":"https://verify.example/session","expiresIn":1800}}`), nil
		case "POST /api/openapi-maas/exp/aicc/v2/real-person-auth/asset-group/by-byted-token":
			return assetJSONResponse(`{"requestId":"request-result","state":"OK","body":"real-group-1"}`), nil
		default:
			t.Fatalf("unexpected CMCC request: %s", key)
			return nil, nil
		}
	}))
	require.NoError(t, err)
	adapter.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	require.NoError(t, adapter.CheckConnectivity(context.Background()))
	asset, err := adapter.GetAsset(context.Background(), "asset-1")
	require.NoError(t, err)
	assert.Equal(t, "asset-1", asset.ReferenceValue)
	assert.NotContains(t, asset.ReferenceValue, "signed.example")
	_, err = adapter.UpdateAsset(context.Background(), "asset-1", "renamed")
	require.NoError(t, err)
	require.NoError(t, adapter.DeleteAsset(context.Background(), "asset-1"))
	group, err := adapter.CreateGroup(context.Background(), GroupRequest{Name: "characters", Description: "AIGC"})
	require.NoError(t, err)
	assert.Equal(t, "group-1", group.ResourceID)
	_, err = adapter.GetGroup(context.Background(), "group-1")
	require.NoError(t, err)
	require.NoError(t, adapter.DeleteGroup(context.Background(), "group-1"))
	session, err := adapter.CreateVerificationSession(context.Background(), VerificationRequest{})
	require.NoError(t, err)
	assert.Equal(t, "opaque-session", session.SessionID)
	assert.Equal(t, int64(1_800_001_800), session.ExpiresAt)
	verification, err := adapter.GetVerificationResult(context.Background(), session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, "real-group-1", verification.GroupID)
	assert.Equal(t, "active", verification.Status)
	assert.Len(t, requests, 9)
}

func TestCMCCAICCV2SignatureMatchesFixedProviderVector(t *testing.T) {
	adapter, err := NewCMCCAICCV2Adapter("ACCESS|SECRET", nil)
	require.NoError(t, err)
	adapter.readNonce = func(target []byte) (int, error) {
		for index := range target {
			target[index] = byte(index)
		}
		return len(target), nil
	}

	query, err := adapter.signedQuery(
		http.MethodPost,
		"/api/openapi-maas/exp/aicc/v2/asset-group/",
		time.Date(2026, time.August, 21, 1, 2, 3, 0, time.UTC),
	)
	require.NoError(t, err)

	assert.Equal(t,
		"AccessKey=ACCESS&Signature=2d19c7ca26d461a67921fae52d60b65eb6138731148f5f0fb897b1e209804815&SignatureMethod=HmacSHA256&SignatureNonce=000102030405060708090a0b0c0d0e0f&SignatureVersion=V2.0&Timestamp=2026-08-21T01%3A02%3A03Z",
		query,
	)
}

func TestCMCCAICCV2CapabilitiesAndErrorBoundary(t *testing.T) {
	adapter, err := NewCMCCAICCV2Adapter("ACCESS|SECRET", nil)
	require.NoError(t, err)
	for _, kind := range []string{"general", "real_person"} {
		for _, mediaType := range []string{"image", "video", "audio"} {
			assert.True(t, adapter.Supports(kind, mediaType))
		}
	}
	assert.False(t, adapter.Supports("unknown", "image"))

	_, err = adapter.CreateVerificationSession(context.Background(), VerificationRequest{RedirectURL: "https://client.example/callback"})
	require.ErrorIs(t, err, ErrAssetOperationUnsupported)

	failed, err := NewCMCCAICCV2Adapter("ACCESS|SECRET", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return assetJSONResponse(`{"requestId":"request-error","state":"ERROR","errorCode":"AUTH_DENIED","errorMessage":"sensitive provider text"}`), nil
	}))
	require.NoError(t, err)
	err = failed.CheckConnectivity(context.Background())
	require.Error(t, err)
	diagnostic, ok := SafeUpstreamDiagnostic(err)
	assert.True(t, ok)
	assert.Equal(t, "provider_code=AUTH_DENIED", diagnostic)
	assert.NotContains(t, err.Error(), "sensitive provider text")
	assert.False(t, strings.Contains(err.Error(), "SECRET"))
}
