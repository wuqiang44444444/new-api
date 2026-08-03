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

func TestOfficialActionAdapterSignsCreateAssetAndUsesOfficialContract(t *testing.T) {
	var captured *http.Request
	var body map[string]any
	adapter, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Clone(req.Context())
			payload, readErr := io.ReadAll(req.Body)
			require.NoError(t, readErr)
			require.NoError(t, common.Unmarshal(payload, &body))
			return assetJSONResponse(`{
				"ResponseMetadata":{"RequestId":"request-1","Action":"CreateAsset","Version":"2024-01-01","Service":"ark","Region":"ap-southeast-1"},
				"Result":{"Id":"asset-1","Status":"Active"}
			}`), nil
		}),
	)
	require.NoError(t, err)
	adapter.now = func() time.Time {
		return time.Date(2026, time.July, 23, 8, 9, 10, 0, time.UTC)
	}

	result, err := adapter.CreateAsset(context.Background(), AssetRequest{
		GroupResourceID: "group-1",
		URL:             "https://blob.example/input.png",
		Name:            "portrait",
		MediaType:       "image",
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, http.MethodPost, captured.Method)
	assert.Contains(t, []string{"", "/"}, captured.URL.Path)
	assert.Equal(t, "CreateAsset", captured.URL.Query().Get("Action"))
	assert.Equal(t, officialActionVersion, captured.URL.Query().Get("Version"))
	assert.Equal(t, "project-a", body["ProjectName"])
	assert.Equal(t, "group-1", body["GroupId"])
	assert.Equal(t, "Image", body["AssetType"])
	assert.Equal(t, "20260723T080910Z", captured.Header.Get("X-Date"))
	assert.Equal(t, captured.Header.Get("X-Content-Sha256"), sha256Hex(mustMarshalOfficialActionBody(t, body)))
	authorization := captured.Header.Get("Authorization")
	assert.Contains(t, authorization, "Credential=ACCESS/20260723/ap-southeast-1/ark/request")
	assert.Contains(t, authorization, "SignedHeaders=content-type;host;x-content-sha256;x-date")
	assert.NotContains(t, authorization, "SECRET")
	assert.Equal(t, "asset-1", result.ResourceID)
	assert.Equal(t, "asset_uri_id", result.ReferenceType)
	assert.Equal(t, "active", result.Status)
}

func TestOfficialActionVerificationUsesEncryptedHandleSourceContract(t *testing.T) {
	actions := make([]string, 0, 2)
	bodies := make([]map[string]any, 0, 2)
	adapter, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
			actions = append(actions, req.URL.Query().Get("Action"))
			payload, readErr := io.ReadAll(req.Body)
			require.NoError(t, readErr)
			var body map[string]any
			require.NoError(t, common.Unmarshal(payload, &body))
			bodies = append(bodies, body)
			if req.URL.Query().Get("Action") == "CreateVisualValidateSession" {
				return assetJSONResponse(`{"BytedToken":"short-lived-token","H5Link":"https://www.byteplus.com/verify"}`), nil
			}
			return assetJSONResponse(`{"Result":{"GroupId":"group-real-person"}}`), nil
		}),
	)
	require.NoError(t, err)
	adapter.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	created, err := adapter.CreateVerificationSession(context.Background(), VerificationRequest{
		RedirectURL: "https://platform.example/verification/real-person/complete",
	})
	require.NoError(t, err)
	assert.Equal(t, "short-lived-token", created.Handle)
	assert.Equal(t, "https://www.byteplus.com/verify", created.H5URL)
	assert.Equal(t, int64(1_800_001_800), created.ExpiresAt)

	result, err := adapter.GetVerificationResult(context.Background(), created.Handle)
	require.NoError(t, err)
	assert.Equal(t, "group-real-person", result.GroupID)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, []string{"CreateVisualValidateSession", "GetVisualValidateResult"}, actions)
	assert.Equal(t, "project-a", bodies[0]["ProjectName"])
	assert.Equal(t, "short-lived-token", bodies[1]["BytedToken"])
	assert.Equal(t, "project-a", bodies[1]["ProjectName"])
}

func TestOfficialActionDeleteGroupUsesVerifiedActionAndNoBearerAuth(t *testing.T) {
	adapter, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "DeleteAssetGroup", req.URL.Query().Get("Action"))
			assert.False(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
			return assetJSONResponse(`{"ResponseMetadata":{"Action":"DeleteAssetGroup"},"Result":{}}`), nil
		}),
	)
	require.NoError(t, err)
	require.NoError(t, adapter.DeleteGroup(context.Background(), "group-1"))
}

func TestOfficialActionAdapterRejectsUnsafeConfiguration(t *testing.T) {
	_, err := NewOfficialActionAdapter("http://ark.example", "ACCESS|SECRET", "region", "project", nil)
	require.Error(t, err)
	_, err = NewOfficialActionAdapter("https://ark.example", "single-value", "region", "project", nil)
	require.Error(t, err)
	_, err = NewOfficialActionAdapter("https://ark.example", "ACCESS|SECRET", "", "project", nil)
	require.Error(t, err)
}

func mustMarshalOfficialActionBody(t *testing.T, body map[string]any) []byte {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	return payload
}
