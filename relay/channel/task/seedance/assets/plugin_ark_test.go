package assets

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkPluginPreservesAssetOperations(t *testing.T) {
	plugin, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	for _, tc := range []struct {
		name, body string
		invoke     func(Adapter) (any, error)
	}{
		{"create", `{"Id":"a","Status":"Active"}`, func(a Adapter) (any, error) {
			return a.CreateAsset(context.Background(), AssetRequest{GroupResourceID: "group", URL: "https://source.example/image.png", Name: "图片", MediaType: " IMAGE "})
		}},
		{"get", `{"Id":"a","Status":"Pending"}`, func(a Adapter) (any, error) { return a.GetAsset(context.Background(), "id/with+space ") }},
		{"failed", `{"Id":"a","Status":"Active","ErrorCode":"rejected","ErrorMessage":"invalid"}`, func(a Adapter) (any, error) { return a.GetAsset(context.Background(), "a") }},
		{"update", `{"Status":"Active"}`, func(a Adapter) (any, error) { return a.UpdateAsset(context.Background(), "a", "renamed") }},
		{"delete", "", func(a Adapter) (any, error) { return nil, a.DeleteAsset(context.Background(), "a") }},
		{"create-group", `{"Id":"g"}`, func(a Adapter) (any, error) {
			return a.(GroupAdapter).CreateGroup(context.Background(), GroupRequest{Name: "group", Description: "description", GroupType: "AIGC"})
		}},
		{"get-group", `{"Id":"g","Status":"Processing"}`, func(a Adapter) (any, error) { return a.(GroupAdapter).GetGroup(context.Background(), "g") }},
		{"create-verification", `{"session_id":"session","h5_link":"https://verify.example","expires_at":1700000000}`, func(a Adapter) (any, error) {
			return a.(VerificationAdapter).CreateVerificationSession(context.Background(), VerificationRequest{RedirectURL: "https://callback.example", ProjectName: "project"})
		}},
		{"get-verification-session", `{"session_id":"s","status":"verifying"}`, func(a Adapter) (any, error) {
			return a.(VerificationAdapter).GetVerificationSession(context.Background(), "s")
		}},
		{"get-verification", `{"group_id":"g","status":"active"}`, func(a Adapter) (any, error) {
			return a.(VerificationAdapter).GetVerificationResult(context.Background(), "s")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests []struct {
				Method, URL, Body string
				Header            http.Header
			}
			client := assetHTTPDoerFunc(func(r *http.Request) (*http.Response, error) {
				var body []byte
				var err error
				if r.Body != nil {
					body, err = io.ReadAll(r.Body)
				}
				require.NoError(t, err)
				requests = append(requests, struct {
					Method, URL, Body string
					Header            http.Header
				}{r.Method, r.URL.String(), string(body), r.Header.Clone()})
				return assetJSONResponse(tc.body), nil
			})
			old := NewArkAdapter("https://provider.example", "fixture-key", client)
			next, err := NewPluginAssetAdapter(plugin, info.Configuration.Asset("ark_assets_v1"), dto.AssetUpstreamProtocolArkAssetsV1, "https://provider.example", "fixture-key", "", "", client)
			require.NoError(t, err)
			expected, err := tc.invoke(old)
			require.NoError(t, err)
			actual, err := tc.invoke(next)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
			require.Len(t, requests, 2)
			assert.Equal(t, requests[0], requests[1])
			assert.False(t, next.CanSearchGroups(), "default group creation must not issue an unsupported search")
		})
	}
}
