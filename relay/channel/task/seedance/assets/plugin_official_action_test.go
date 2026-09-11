package assets

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each action is compared across the production plugin bridge and the frozen
// Go baseline, including body bytes, signed headers and normalized results.
func TestPluginOfficialActionsPreserveWireSignatureAndResults(t *testing.T) {
	plugin, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	ctx := context.Background()
	for _, tc := range []struct {
		name, body string
		invoke     func(*OfficialActionAdapter, *PluginAssetAdapter) (any, error, any, error)
	}{
		{"create", `{"Result":{"Id":"asset-1","Status":"Active"}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			input := AssetRequest{GroupResourceID: "group-1", URL: "https://source.example/image.png", Name: "肖像", MediaType: " IMAGE "}
			a, b := old.CreateAsset(ctx, input)
			c, d := next.CreateAsset(ctx, input)
			return a, b, c, d
		}},
		{"get", `{"Result":{"Id":"asset-1","Status":"Processing"}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			a, b := old.GetAsset(ctx, "asset-1")
			c, d := next.GetAsset(ctx, "asset-1")
			return a, b, c, d
		}},
		{"update", `{"Result":{"Status":"Active"}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			a, b := old.UpdateAsset(ctx, "asset-1", "renamed")
			c, d := next.UpdateAsset(ctx, "asset-1", "renamed")
			return a, b, c, d
		}},
		{"delete", `{}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			return nil, old.DeleteAsset(ctx, "asset-1"), nil, next.DeleteAsset(ctx, "asset-1")
		}},
		{"create-group", `{"Result":{"Id":"group-1","Name":"group","Status":""}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			input := GroupRequest{Name: "group", Description: "说明"}
			a, b := old.CreateGroup(ctx, input)
			c, d := next.CreateGroup(ctx, input)
			return a, b, c, d
		}},
		{"get-group", `{"Result":{"Id":"group-1","Status":"Failed"}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			a, b := old.GetGroup(ctx, "group-1")
			c, d := next.GetGroup(ctx, "group-1")
			return a, b, c, d
		}},
		{"list-assets", `{"Result":{"Items":[{"Id":"a","Status":"Active"}],"TotalCount":9}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			input := AssetListRequest{GroupType: " AIGC ", GroupIDs: []string{"g"}, Statuses: []string{"Active"}, Name: " name ", Page: 0, PageSize: -1}
			a, b, e := old.ListAssets(ctx, input)
			c, d, f := next.ListAssets(ctx, input)
			return []any{a, b}, e, []any{c, d}, f
		}},
		{"list-groups", `{"Result":{"Items":[],"TotalCount":0}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			input := GroupListRequest{GroupType: " AIGC ", Page: 2, PageSize: 5}
			a, b, e := old.ListGroups(ctx, input)
			c, d, f := next.ListGroups(ctx, input)
			return []any{a, b}, e, []any{c, d}, f
		}},
		{"create-verification", `{"Result":{"BytedToken":"session-1","H5Link":"https://verify.example/"}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			input := VerificationRequest{RedirectURL: "https://client.example/verified"}
			a, b := old.CreateVerificationSession(ctx, input)
			c, d := next.CreateVerificationSession(ctx, input)
			return a, b, c, d
		}},
		{"get-verification", `{"Result":{"GroupId":"verified-group"}}`, func(old *OfficialActionAdapter, next *PluginAssetAdapter) (any, error, any, error) {
			a, b := old.GetVerificationResult(ctx, "session-1")
			c, d := next.GetVerificationResult(ctx, "session-1")
			return a, b, c, d
		}},
	} {
		for _, protocol := range []dto.AssetUpstreamProtocol{dto.AssetUpstreamProtocolVolcengineAction, dto.AssetUpstreamProtocolBytePlusAction} {
			t.Run(tc.name+"/"+string(protocol), func(t *testing.T) {
				var requests []struct {
					URL, Body string
					Headers   http.Header
				}
				client := assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					requests = append(requests, struct {
						URL, Body string
						Headers   http.Header
					}{req.URL.String(), string(body), req.Header.Clone()})
					return assetJSONResponse(tc.body), nil
				})
				region := "cn-beijing"
				old, err := NewVolcengineActionAdapter("fixture-access|fixture-secret", "project-a", client)
				if protocol == dto.AssetUpstreamProtocolBytePlusAction {
					region = "ap-southeast-1"
					old, err = NewBytePlusActionAdapter("fixture-access|fixture-secret", region, "project-a", client)
				}
				require.NoError(t, err)
				next, err := NewPluginAssetAdapter(plugin, info.Configuration.Asset(string(protocol)), protocol, "", "fixture-access|fixture-secret", region, "project-a", client)
				require.NoError(t, err)
				now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
				old.now = now
				next.connection.now = now
				expected, oldErr, actual, newErr := tc.invoke(old, next)
				require.NoError(t, oldErr)
				require.NoError(t, newErr)
				assert.Equal(t, expected, actual)
				require.Len(t, requests, 2)
				assert.Equal(t, requests[0], requests[1])
				assert.NotContains(t, requests[1].Body, "fixture-secret")
			})
		}
	}
}

func TestPluginOfficialActionFailureAndOperationBoundary(t *testing.T) {
	plugin, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	for _, tc := range []struct {
		name, body string
		status     int
		definitive bool
	}{
		{"http-error", `{"ResponseMetadata":{"Error":{"Code":"InvalidParameter","CodeN":40001}}}`, 400, true},
		{"non-json-http", `<html>gateway failure</html>`, 503, false},
		{"application-error", `{"ResponseMetadata":{"Error":{"CodeN":40001}}}`, 200, true},
		{"unsafe-code", `{"ResponseMetadata":{"Error":{"Code":"https://private.example/?token=secret"}}}`, 400, true},
		{"invalid-result", `{"Result":[1]}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, err := NewPluginAssetAdapter(plugin, info.Configuration.Asset(string(dto.AssetUpstreamProtocolVolcengineAction)), dto.AssetUpstreamProtocolVolcengineAction, "", "access|secret", "cn-beijing", "project", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				response := assetJSONResponse(tc.body)
				response.StatusCode = tc.status
				return response, nil
			}))
			require.NoError(t, err)
			_, err = next.GetAsset(context.Background(), "asset-1")
			require.Error(t, err)
			assert.Equal(t, tc.definitive, IsDefinitiveUpstreamRejection(err))
			assert.NotContains(t, err.Error(), "token=secret")
		})
	}
}
