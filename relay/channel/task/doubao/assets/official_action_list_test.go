package assets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialActionListContractsRequireAndSendGroupType(t *testing.T) {
	type capturedCall struct {
		action string
		body   map[string]any
	}
	calls := make([]capturedCall, 0, 3)
	adapter, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
			payload, readErr := io.ReadAll(req.Body)
			require.NoError(t, readErr)
			var body map[string]any
			require.NoError(t, common.Unmarshal(payload, &body))
			calls = append(calls, capturedCall{action: req.URL.Query().Get("Action"), body: body})
			return assetJSONResponse(`{"Result":{"Items":[],"TotalCount":0}}`), nil
		}),
	)
	require.NoError(t, err)

	_, _, err = adapter.ListAssets(context.Background(), AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 100})
	require.NoError(t, err)
	_, _, err = adapter.ListGroups(context.Background(), GroupListRequest{GroupType: "LivenessFace", Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.NoError(t, adapter.CheckConnectivity(context.Background()))

	require.Len(t, calls, 3)
	assert.Equal(t, "ListAssets", calls[0].action)
	assert.Equal(t, "AIGC", calls[0].body["Filter"].(map[string]any)["GroupType"])
	assert.Equal(t, "project-a", calls[0].body["ProjectName"])
	assert.Equal(t, float64(100), calls[0].body["PageSize"])
	assert.Equal(t, "ListAssetGroups", calls[1].action)
	assert.Equal(t, "LivenessFace", calls[1].body["Filter"].(map[string]any)["GroupType"])
	assert.Equal(t, "AIGC", calls[2].body["Filter"].(map[string]any)["GroupType"])
}

func TestOfficialActionListsRejectMissingGroupTypeBeforeCallingUpstream(t *testing.T) {
	called := false
	adapter, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return assetJSONResponse(`{}`), nil
		}),
	)
	require.NoError(t, err)

	_, _, assetErr := adapter.ListAssets(context.Background(), AssetListRequest{Page: 1, PageSize: 1})
	_, _, groupErr := adapter.ListGroups(context.Background(), GroupListRequest{Page: 1, PageSize: 1})

	require.Error(t, assetErr)
	require.Error(t, groupErr)
	assert.False(t, called)
}

func TestOfficialActionHTTPErrorKeepsSafeProviderCode(t *testing.T) {
	adapter, err := NewOfficialActionAdapter(
		"https://ark.ap-southeast-1.byteplusapi.com",
		"ACCESS|SECRET",
		"ap-southeast-1",
		"project-a",
		assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body: io.NopCloser(strings.NewReader(`{
					"ResponseMetadata":{
						"Error":{
							"Code":"MissingParameter.Filter.GroupType",
							"Message":"private upstream diagnostic"
						}
					}
				}`)),
			}, nil
		}),
	)
	require.NoError(t, err)

	_, _, err = adapter.ListAssets(context.Background(), AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})

	require.Error(t, err)
	assert.Equal(t, "asset upstream returned HTTP 400 (MissingParameter.Filter.GroupType)", err.Error())
	assert.NotContains(t, err.Error(), "private upstream diagnostic")

	unsafeCodeErr := officialActionHTTPError(
		http.StatusBadRequest,
		[]byte(`{"ResponseMetadata":{"Error":{"Code":"private diagnostic: secret-token"}}}`),
	)
	assert.Equal(t, "asset upstream returned HTTP 400", unsafeCodeErr.Error())
	assert.NotContains(t, unsafeCodeErr.Error(), "secret-token")
}
