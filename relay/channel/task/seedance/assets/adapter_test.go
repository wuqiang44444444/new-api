// Package assets implements Seedance asset-library protocols.
package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestMoxingAssetAdaptersUseProviderSpecificContracts(t *testing.T) {
	tests := []struct {
		name      string
		adapter   Adapter
		wantPath  string
		wantID    string
		wantRefID string
	}{
		{
			name: "JoyCreator", wantPath: "/joycreator/openApi/v1/asset/create", wantID: "52", wantRefID: "asset-joy-1",
			adapter: NewMoxingJoyCreatorAdapter("https://moxing.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				return assetJSONResponse(`{"requestId":"request-1","error":null,"result":{"asset":{"id":"52","assetId":"asset-joy-1","vendorStatus":"Active","status":1}}}`), nil
			})),
		},
		{
			name: "Volcengine", wantPath: "/v1/volc/assets", wantID: "asset-volc-1", wantRefID: "asset-volc-1",
			adapter: NewMoxingVolcAdapter("https://moxing.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				return assetJSONResponse(`{"RequestId":"request-2","Result":{"Id":"asset-volc-1","Status":"Active"}}`), nil
			})),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestedPath string
			switch adapter := test.adapter.(type) {
			case *MoxingJoyCreatorAdapter:
				original := adapter.http
				adapter.http = assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
					requestedPath = req.URL.Path
					return original.Do(req)
				})
			case *MoxingVolcAdapter:
				original := adapter.http
				adapter.http = assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
					requestedPath = req.URL.Path
					return original.Do(req)
				})
			}
			result, err := test.adapter.CreateAsset(context.Background(), AssetRequest{
				GroupResourceID: "group-1", URL: "https://blob.example/source.png", Name: "source", MediaType: "image",
			})
			require.NoError(t, err)
			assert.Equal(t, test.wantPath, requestedPath)
			assert.Equal(t, test.wantID, result.ResourceID)
			assert.Equal(t, test.wantRefID, result.ReferenceValue)
			assert.Equal(t, "active", result.Status)
		})
	}
}

func TestMoxingAndBytePlusImplementUnifiedRealPersonContract(t *testing.T) {
	moxing := NewArkAdapter("https://tokensave.pro", "moxing-key", nil)
	bytePlus, err := NewBytePlusActionAdapter(
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
		{
			name:     "Moxing Volcengine assets",
			wantPath: "/v1/volc/assets/list",
			adapter: func(client HTTPDoer) ConnectivityAdapter {
				return NewMoxingVolcAdapter("https://upstream.example", "channel-key", client)
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

func TestArkGroupDeleteFailsClosedWhenExactEndpointIsUnconfirmed(t *testing.T) {
	adapter := NewArkAdapter("https://upstream.example", "key", nil)
	err := adapter.DeleteGroup(context.Background(), "group-1")
	require.ErrorIs(t, err, ErrGroupDeletionUnsupported)
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

func TestMoxingJoyCreatorApplicationErrorsPreserveUnknownServerOutcomes(t *testing.T) {
	for _, test := range []struct {
		code       int
		definitive bool
	}{
		{code: 400, definitive: true},
		{code: 500, definitive: false},
	} {
		adapter := NewMoxingJoyCreatorAdapter("https://moxing.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return assetJSONResponse(fmt.Sprintf(`{"requestId":"request-1","error":{"code":%d,"message":"failed"},"result":{}}`, test.code)), nil
		}))
		_, err := adapter.CreateAsset(context.Background(), AssetRequest{URL: "https://example.com/source.png", MediaType: "image"})
		require.Error(t, err)
		assert.Equal(t, test.definitive, IsDefinitiveUpstreamRejection(err))
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

func TestAdapterDeletionIsIdempotent(t *testing.T) {
	relayAdapter := NewRelayAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"detail":"missing"}`))}, nil
	}))
	require.NoError(t, relayAdapter.DeleteAsset(context.Background(), "missing"))
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

func TestUpstreamNotFoundIsExplicitlyClassified(t *testing.T) {
	adapter := NewRelayAdapter("https://upstream.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"detail":"missing"}`)),
		}, nil
	}))
	_, err := adapter.GetAsset(context.Background(), "missing")
	require.Error(t, err)
	assert.True(t, IsUpstreamNotFound(err))
}

func TestFunCloudMaterialAdapterUsesPublishedGroupAndVirtualUploadContracts(t *testing.T) {
	paths := make([]string, 0, 4)
	client := assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.Method+" "+req.URL.RequestURI())
		assert.Equal(t, "Bearer funcloud-key", req.Header.Get("Authorization"))
		switch req.URL.Path {
		case "/api/v2/open/material/group/create":
			return assetJSONResponse(`{"code":0,"data":{"groupId":"group-1"}}`), nil
		case "/api/v2/open/material/group/list":
			return assetJSONResponse(`{"code":0,"data":{"list":[{"groupId":"group-1","materialCount":0}]}}`), nil
		case "/api/v2/open/material/group/delete":
			return assetJSONResponse(`{"code":0,"data":{}}`), nil
		case "/api/v2/open/material/virtual/upload":
			multipartReader, err := req.MultipartReader()
			require.NoError(t, err)
			fields := map[string]string{}
			for {
				part, partErr := multipartReader.NextPart()
				if partErr == io.EOF {
					break
				}
				require.NoError(t, partErr)
				value, readErr := io.ReadAll(part)
				require.NoError(t, readErr)
				if part.FormName() == "file" {
					assert.Equal(t, "video/mp4", part.Header.Get("Content-Type"))
				}
				fields[part.FormName()] = string(value)
			}
			assert.Equal(t, "video-bytes", fields["file"])
			assert.Equal(t, "group-1", fields["groupId"])
			assert.Equal(t, "clip", fields["materialName"])
			return assetJSONResponse(`{"code":0,"data":{"materialId":"material-1","assetUrl":"asset://provider-asset-1","assetStatus":"active"}}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", req.URL.RequestURI())
		}
	})
	adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "funcloud-key", client)
	assert.Equal(t, "funcloud_material", string(adapter.Profile()))
	assert.True(t, adapter.Supports("general", "image"))
	assert.False(t, adapter.Supports("real_person", "image"))

	group, err := adapter.CreateGroup(context.Background(), GroupRequest{Name: "group", Description: "description"})
	require.NoError(t, err)
	assert.Equal(t, "group-1", group.ResourceID)
	require.NoError(t, adapter.DeleteGroup(context.Background(), "group-1"))

	asset, err := adapter.CreateAsset(context.Background(), AssetRequest{
		GroupResourceID: "group-1", Name: "clip", MediaType: "video",
		Source: bytes.NewBufferString("video-bytes"), SourceFilename: "upload.mp4", SourceMaxBytes: 100,
		SourceType: "video/mp4",
	})
	require.NoError(t, err)
	assert.Equal(t, "material-1", asset.ResourceID)
	assert.Equal(t, "asset_uri_id", asset.ReferenceType)
	assert.Equal(t, "provider-asset-1", asset.ReferenceValue)
	assert.Equal(t, "active", asset.Status)
	assert.Equal(t, []string{
		"POST /api/v2/open/material/group/create",
		"GET /api/v2/open/material/group/list?page=1&pageSize=100",
		"POST /api/v2/open/material/group/delete?groupId=group-1",
		"POST /api/v2/open/material/virtual/upload",
	}, paths)

	_, err = adapter.UpdateAsset(context.Background(), "material-1", "new-name")
	require.ErrorIs(t, err, ErrAssetOperationUnsupported)
	require.ErrorIs(t, adapter.DeleteAsset(context.Background(), "material-1"), ErrAssetOperationUnsupported)
}

func TestFunCloudMaterialGroupDeleteFailsClosedUnlessProviderGroupIsEmpty(t *testing.T) {
	tests := []struct {
		name         string
		listResponse *http.Response
		listErr      error
		wantNotEmpty bool
		wantDelete   bool
		wantNoError  bool
	}{
		{
			name: "non-empty group", listResponse: assetJSONResponse(`{"code":0,"data":{"list":[{"groupId":"group-1","materialCount":2}]}}`),
			wantNotEmpty: true,
		},
		{
			name: "empty group", listResponse: assetJSONResponse(`{"code":0,"data":{"list":[{"groupId":"group-1","materialCount":0}]}}`),
			wantDelete: true, wantNoError: true,
		},
		{
			name: "missing group", listResponse: assetJSONResponse(`{"code":0,"data":{"list":[]}}`),
			wantNoError: true,
		},
		{
			name: "ambiguous envelope", listResponse: assetJSONResponse(`{"code":0,"data":{"list":[],"items":[]}}`),
		},
		{
			name: "missing material count", listResponse: assetJSONResponse(`{"code":0,"data":{"list":[{"groupId":"group-1"}]}}`),
		},
		{
			name: "lookup transport failure", listErr: fmt.Errorf("lookup unavailable"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deleteCalled := false
			adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/v2/open/material/group/list":
					return test.listResponse, test.listErr
				case "/api/v2/open/material/group/delete":
					deleteCalled = true
					return assetJSONResponse(`{"code":0,"data":{}}`), nil
				default:
					return nil, fmt.Errorf("unexpected request %s", req.URL.Path)
				}
			}))

			err := adapter.DeleteGroup(context.Background(), "group-1")
			if test.wantNoError {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, test.wantNotEmpty, errors.Is(err, ErrGroupNotEmpty))
			assert.Equal(t, test.wantDelete, deleteCalled)
		})
	}
}

func TestFunCloudMaterialListAndAssetNormalizationFailClosed(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "no collection", data: `{}`},
		{name: "multiple collections", data: `{"list":[],"records":[]}`},
		{name: "conflicting asset ids", data: `[{"materialId":"material-1","assetStatus":"processing"},{"materialId":"material-1","assetStatus":"processing"}]`},
		{name: "invalid active asset URL", data: `[{"materialId":"material-1","assetStatus":"active","assetUrl":"https://private.example/material"}]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				return assetJSONResponse(`{"code":0,"data":` + test.data + `}`), nil
			}))
			_, err := adapter.GetAsset(context.Background(), "material-1")
			require.Error(t, err)
		})
	}
}

func TestFunCloudMaterialConnectivityUsesReadOnlyListAndRejectsApplicationErrors(t *testing.T) {
	requestedURI := ""
	adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		requestedURI = req.URL.RequestURI()
		return assetJSONResponse(`{"code":10002,"msg":"invalid"}`), nil
	}))
	require.Error(t, adapter.CheckConnectivity(context.Background()))
	assert.Equal(t, "/api/v2/open/material/list?page=1&pageSize=1", requestedURI)
}

func TestFunCloudMaterialUploadEnforcesStreamingLimit(t *testing.T) {
	adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		_, err := io.Copy(io.Discard, req.Body)
		return nil, err
	}))
	_, err := adapter.CreateAsset(context.Background(), AssetRequest{
		GroupResourceID: "group-1", Name: "oversized", SourceFilename: "upload.png",
		Source: bytes.NewBufferString("too-large"), SourceType: "image/png", SourceMaxBytes: 3,
	})
	require.ErrorContains(t, err, "exceeds upload limit")
}

func TestFunCloudMaterialUploadRequiresGroup(t *testing.T) {
	adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "key", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request must not be sent without a group")
		return nil, nil
	}))
	_, err := adapter.CreateAsset(context.Background(), AssetRequest{
		Name: "source", SourceFilename: "upload.png", Source: bytes.NewBufferString("image"),
		SourceType: "image/png", SourceMaxBytes: 10,
	})
	require.ErrorContains(t, err, "source and group are required")
}

func TestFunCloudMaterialUploadRejectsOversizedResponse(t *testing.T) {
	adapter := NewFunCloudMaterialAdapter("https://funcloud.example", "key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		_, err := io.Copy(io.Discard, req.Body)
		require.NoError(t, err)
		return assetJSONResponse(strings.Repeat("x", funCloudUploadResponseMaxBytes+1)), nil
	}))
	_, err := adapter.CreateAsset(context.Background(), AssetRequest{
		GroupResourceID: "group-1", Name: "source", SourceFilename: "upload.png",
		Source: bytes.NewBufferString("image"), SourceType: "image/png", SourceMaxBytes: 10,
	})
	require.ErrorContains(t, err, "response is invalid")
}
