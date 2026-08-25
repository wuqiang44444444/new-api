// Package assets implements Seedance asset-library protocols.
package assets

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialActionAdapterSignsCreateAssetAndUsesOfficialContract(t *testing.T) {
	var captured *http.Request
	var body map[string]any
	adapter, err := NewBytePlusActionAdapter(
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

func TestOfficialActionConstructorsUseSeparateProviderEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		wantHost string
		create   func(HTTPDoer) (*OfficialActionAdapter, error)
	}{
		{
			name:     "Volcengine domestic",
			wantHost: "ark.cn-beijing.volcengineapi.com",
			create: func(client HTTPDoer) (*OfficialActionAdapter, error) {
				return NewVolcengineActionAdapter("ACCESS|SECRET", "project-cn", client)
			},
		},
		{
			name:     "BytePlus overseas",
			wantHost: "ark.ap-southeast-1.byteplusapi.com",
			create: func(client HTTPDoer) (*OfficialActionAdapter, error) {
				return NewBytePlusActionAdapter("ACCESS|SECRET", "ap-southeast-1", "project-global", client)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var capturedHost string
			adapter, err := test.create(assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				capturedHost = req.URL.Host
				return assetJSONResponse(`{"ResponseMetadata":{},"Result":{"Items":[],"TotalCount":0}}`), nil
			}))
			require.NoError(t, err)

			_, _, err = adapter.ListAssets(context.Background(), AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})

			require.NoError(t, err)
			assert.Equal(t, test.wantHost, capturedHost)
		})
	}
}

func TestOfficialActionVerificationUsesOpaqueProviderSession(t *testing.T) {
	actions := make([]string, 0, 2)
	bodies := make([]map[string]any, 0, 2)
	adapter, err := NewBytePlusActionAdapter(
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
		RedirectURL: "https://client.example/verification/complete",
	})
	require.NoError(t, err)
	assert.Equal(t, "short-lived-token", created.SessionID)
	assert.Equal(t, "https://www.byteplus.com/verify", created.H5URL)
	assert.Equal(t, int64(1_800_001_800), created.ExpiresAt)

	result, err := adapter.GetVerificationResult(context.Background(), created.SessionID)
	require.NoError(t, err)
	assert.Equal(t, "group-real-person", result.GroupID)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, []string{"CreateVisualValidateSession", "GetVisualValidateResult"}, actions)
	assert.Equal(t, "project-a", bodies[0]["ProjectName"])
	assert.Equal(t, "short-lived-token", bodies[1]["BytedToken"])
	assert.Equal(t, "project-a", bodies[1]["ProjectName"])
}

func TestOfficialActionAdapterRejectsInvalidConfiguration(t *testing.T) {
	_, err := NewBytePlusActionAdapter("single-value", "region", "project", nil)
	require.Error(t, err)
	_, err = NewBytePlusActionAdapter("ACCESS|SECRET", "", "project", nil)
	require.Error(t, err)
	_, err = NewVolcengineActionAdapter("ACCESS|SECRET", "", nil)
	require.Error(t, err)
}

func mustMarshalOfficialActionBody(t *testing.T, body map[string]any) []byte {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	return payload
}
