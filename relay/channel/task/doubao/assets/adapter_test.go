package assets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type assetHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f assetHTTPDoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func assetJSONResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}
}

func TestProtocolAdaptersNormalizeCreationContracts(t *testing.T) {
	tests := []struct {
		name        string
		adapter     Adapter
		wantPath    string
		wantStatus  string
		wantID      string
		wantRefType string
	}{
		{
			name: "ark", wantPath: "/v1/ark/assets", wantStatus: "active", wantID: "asset-ark", wantRefType: "asset_uri_id",
			adapter: NewArkAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				return assetJSONResponse(`{"Id":"asset-ark","Status":"Active"}`), nil
			})),
		},
		{
			name: "relay", wantPath: "/assets", wantStatus: "active", wantID: "uuid-1", wantRefType: "asset_uri_id",
			adapter: NewRelayAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				return assetJSONResponse(`{"uuid":"uuid-1","upstream_id":"asset-relay","status":"Active"}`), nil
			})),
		},
		{
			name: "joycreator", wantPath: "/joycreator/openApi/v1/asset/create", wantStatus: "active", wantID: "52", wantRefType: "",
			adapter: NewJoyCreatorAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				return assetJSONResponse(`{"requestId":"req-1","error":null,"result":{"asset":{"id":"52","assetId":"business-52","vendorUrl":"https://cdn.example/52","vendorStatus":"Active","status":1}}}`), nil
			})),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requestedPath string
			switch adapter := tc.adapter.(type) {
			case *ArkAdapter:
				original := adapter.http
				adapter.http = assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) { requestedPath = req.URL.Path; return original.Do(req) })
			case *RelayAdapter:
				original := adapter.http
				adapter.http = assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) { requestedPath = req.URL.Path; return original.Do(req) })
			case *JoyCreatorAdapter:
				original := adapter.http
				adapter.http = assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) { requestedPath = req.URL.Path; return original.Do(req) })
			}
			result, err := tc.adapter.CreateAsset(context.Background(), AssetRequest{GroupResourceID: "group-1", URL: "https://blob.example/source", Name: "asset", MediaType: "image"})
			require.NoError(t, err)
			assert.Equal(t, tc.wantPath, requestedPath)
			assert.Equal(t, tc.wantStatus, result.Status)
			assert.Equal(t, tc.wantID, result.ResourceID)
			assert.Equal(t, tc.wantRefType, result.ReferenceType)
		})
	}
}

func TestMoxingAndBytePlusImplementUnifiedRealPersonContract(t *testing.T) {
	moxing := NewArkAdapter("https://tokensave.pro", "moxing-key", nil)
	bytePlus, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		nil,
	)
	require.NoError(t, err)

	for _, adapter := range []VerificationAdapter{moxing, bytePlus} {
		assert.True(t, adapter.Supports("real_person", "image"))
		assert.False(t, adapter.Supports("real_person", "video"))
		assert.False(t, adapter.Supports("real_person", "audio"))
	}
}

func TestProxyConnectivityUsesDocumentedReadOnlyListEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		wantPath string
		adapter  func(HTTPDoer) ConnectivityAdapter
	}{
		{
			name:     "ark assets",
			wantPath: "/v1/ark/assets/list",
			adapter: func(client HTTPDoer) ConnectivityAdapter {
				return NewArkAdapter("https://upstream.example", "channel-key", client)
			},
		},
		{
			name:     "relay assets",
			wantPath: "/assets/list",
			adapter: func(client HTTPDoer) ConnectivityAdapter {
				return NewRelayAdapter("https://upstream.example", "channel-key", client)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, test.wantPath, req.URL.Path)
				assert.Equal(t, "Bearer channel-key", req.Header.Get("Authorization"))
				return assetJSONResponse(`{}`), nil
			})

			require.NoError(t, test.adapter(client).CheckConnectivity(context.Background()))
		})
	}
}

func TestJoyCreatorGroupDeleteFailsClosedWhenProtocolIsUndocumented(t *testing.T) {
	adapter := NewJoyCreatorAdapter("https://upstream.example", "key", nil)
	err := adapter.DeleteGroup(context.Background(), "34")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not documented")
}

func TestArkGroupDeleteFailsClosedWhenExactEndpointIsUnconfirmed(t *testing.T) {
	adapter := NewArkAdapter("https://upstream.example", "key", nil)
	err := adapter.DeleteGroup(context.Background(), "group-1")
	require.ErrorIs(t, err, ErrGroupDeletionUnsupported)
}

func TestJoyCreatorAdapterImplementsAllSevenDocumentedEndpoints(t *testing.T) {
	type call struct{ method, path string }
	calls := make([]call, 0, 7)
	adapter := NewJoyCreatorAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, call{method: req.Method, path: req.URL.Path})
		switch {
		case strings.Contains(req.URL.Path, "/group/"):
			return assetJSONResponse(`{"requestId":"group-request","error":null,"result":{"group":{"id":"34","groupId":"group-business","status":1}}}`), nil
		case req.Method == http.MethodDelete:
			return assetJSONResponse(`{}`), nil
		default:
			return assetJSONResponse(`{"requestId":"asset-request","error":null,"result":{"asset":{"id":"52","assetId":"asset-business","vendorUrl":"https://cdn.example/asset","vendorStatus":"Active","status":1}}}`), nil
		}
	}))

	_, err := adapter.CreateGroup(context.Background(), GroupRequest{Name: "group", GroupType: "AIGC"})
	require.NoError(t, err)
	_, err = adapter.GetGroup(context.Background(), "34")
	require.NoError(t, err)
	_, err = adapter.UpdateGroup(context.Background(), "34", GroupRequest{Name: "renamed"})
	require.NoError(t, err)
	_, err = adapter.CreateAsset(context.Background(), AssetRequest{GroupResourceID: "34", URL: "https://blob.example/source", Name: "asset", MediaType: "image"})
	require.NoError(t, err)
	_, err = adapter.GetAsset(context.Background(), "52")
	require.NoError(t, err)
	_, err = adapter.UpdateAsset(context.Background(), "52", "renamed")
	require.NoError(t, err)
	require.NoError(t, adapter.DeleteAsset(context.Background(), "52"))

	assert.Equal(t, []call{
		{http.MethodPost, "/joycreator/openApi/v1/asset/group/create"},
		{http.MethodPost, "/joycreator/openApi/v1/asset/group/detail/34"},
		{http.MethodPost, "/joycreator/openApi/v1/asset/group/34"},
		{http.MethodPost, "/joycreator/openApi/v1/asset/create"},
		{http.MethodPost, "/joycreator/openApi/v1/asset/detail/52"},
		{http.MethodPost, "/joycreator/openApi/v1/asset/52"},
		{http.MethodDelete, "/joycreator/openApi/v1/asset/52"},
	}, calls)
}

func TestJoyCreatorApplicationErrorIsDefinitiveCreateRejection(t *testing.T) {
	adapter := NewJoyCreatorAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return assetJSONResponse(`{"requestId":"request","error":{"code":40001,"message":"rejected"},"result":{}}`), nil
	}))
	_, err := adapter.CreateAsset(context.Background(), AssetRequest{URL: "https://source.example/asset.png", MediaType: "image"})
	require.Error(t, err)
	assert.True(t, IsDefinitiveUpstreamRejection(err))
}

func TestCreateErrorClassificationSeparatesRejectedFromUnknownOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		definitive bool
	}{
		{name: "bad request", err: &upstreamHTTPError{StatusCode: http.StatusBadRequest}, definitive: true},
		{name: "unauthorized", err: &upstreamHTTPError{StatusCode: http.StatusUnauthorized}, definitive: true},
		{name: "unprocessable", err: &upstreamHTTPError{StatusCode: http.StatusUnprocessableEntity}, definitive: true},
		{name: "request timeout", err: &upstreamHTTPError{StatusCode: http.StatusRequestTimeout}, definitive: false},
		{name: "too early", err: &upstreamHTTPError{StatusCode: http.StatusTooEarly}, definitive: false},
		{name: "rate limited", err: &upstreamHTTPError{StatusCode: http.StatusTooManyRequests}, definitive: false},
		{name: "bad gateway", err: &upstreamHTTPError{StatusCode: http.StatusBadGateway}, definitive: false},
		{name: "provider client code", err: &upstreamApplicationError{provider: "provider", code: 40001}, definitive: true},
		{name: "provider server code", err: &upstreamApplicationError{provider: "provider", code: 50001}, definitive: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.definitive, IsDefinitiveUpstreamRejection(tc.err))
		})
	}
}

func TestArkVerificationAdapterUsesDocumentedSessionAndResultEndpoints(t *testing.T) {
	paths := make([]string, 0, 3)
	adapter := NewArkAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.Method+" "+req.URL.Path)
		return assetJSONResponse(`{"session_id":"session-1","group_id":"group-1","h5_link":"https://upstream.example/h5","status":"group_ready"}`), nil
	}))

	created, err := adapter.CreateVerificationSession(context.Background(), VerificationRequest{RedirectURL: "https://api.example/complete", ProjectName: "project"})
	require.NoError(t, err)
	assert.Equal(t, "session-1", created.SessionID)
	_, err = adapter.GetVerificationSession(context.Background(), "session-1")
	require.NoError(t, err)
	result, err := adapter.GetVerificationResult(context.Background(), "session-1")
	require.NoError(t, err)
	assert.Equal(t, "group-1", result.GroupID)
	assert.Equal(t, []string{
		"POST /v1/ark/assets/visual-validate/session",
		"GET /v1/ark/assets/visual-validate/sessions/session-1",
		"GET /v1/ark/assets/visual-validate/result/session-1",
	}, paths)
}

func TestAdapterDeletionIsIdempotentAndJoyCreatorChecksEnvelopeErrors(t *testing.T) {
	relayAdapter := NewRelayAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"detail":"missing"}`))}, nil
	}))
	require.NoError(t, relayAdapter.DeleteAsset(context.Background(), "missing"))

	joyAdapter := NewJoyCreatorAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		return assetJSONResponse(`{"requestId":"request-1","error":{"code":500,"message":"internal detail"},"result":{}}`), nil
	}))
	err := joyAdapter.DeleteAsset(context.Background(), "52")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "internal detail")
	assert.Contains(t, err.Error(), "request-1")
}

func TestUpstreamHTTPErrorDoesNotExposeResponseBody(t *testing.T) {
	adapter := NewRelayAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(`{"sas":"secret-token","detail":"internal"}`))}, nil
	}))
	_, err := adapter.GetAsset(context.Background(), "asset-1")
	require.Error(t, err)
	assert.Equal(t, "asset upstream returned HTTP 502", err.Error())
	assert.NotContains(t, err.Error(), "secret-token")
}
