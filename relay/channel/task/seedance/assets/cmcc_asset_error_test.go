package assets

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCMCCMissingAssetErrorsStayScopedToVerifiedOperations(t *testing.T) {
	const missingGet = `{"state":"ERROR","errorCode":"C500999","errorMessage":"NotFound.asset_id"}`
	const missingDelete = `{"state":"ERROR","errorCode":"C400999","errorMessage":"素材不存在或无权限访问"}`
	for _, tc := range []struct {
		name      string
		operation string
		status    int
		body      string
		notFound  bool
	}{
		{"missing asset query", "get", 500, missingGet, true},
		{"missing or inaccessible asset delete", "delete", 400, missingDelete, true},
		{"same query code different error", "get", 500, `{"state":"ERROR","errorCode":"C500999","errorMessage":"internal error fixture-secret"}`, false},
		{"same delete code different error", "delete", 400, `{"state":"ERROR","errorCode":"C400999","errorMessage":"invalid parameter fixture-secret"}`, false},
		{"different error code", "get", 500, `{"state":"ERROR","errorCode":"OTHER","errorMessage":"NotFound.asset_id"}`, false},
		{"different envelope state", "get", 500, `{"state":"OK","errorCode":"C500999","errorMessage":"NotFound.asset_id"}`, false},
		{"different HTTP status", "get", 503, missingGet, false},
		{"different operation", "get", 400, missingDelete, false},
		{"group query excluded", "group", 500, missingGet, false},
		{"malformed body", "delete", 400, `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := NewCMCCAICCV2Adapter("ACCESS|SECRET", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				response := assetJSONResponse(tc.body)
				response.StatusCode = tc.status
				return response, nil
			}))
			require.NoError(t, err)
			switch tc.operation {
			case "get":
				_, err = adapter.GetAsset(context.Background(), "asset-1")
			case "delete":
				err = adapter.DeleteAsset(context.Background(), "asset-1")
			case "group":
				_, err = adapter.GetGroup(context.Background(), "group-1")
			}
			require.Error(t, err)
			assert.Equal(t, tc.notFound, IsUpstreamNotFound(err))
			assert.NotContains(t, err.Error(), "fixture-secret")
		})
	}
}
