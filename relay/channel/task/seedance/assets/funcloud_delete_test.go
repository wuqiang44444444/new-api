package assets

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudSingleMaterialDelete(t *testing.T) {
	for _, test := range []struct {
		name     string
		body     string
		notFound bool
		success  bool
	}{
		{"deleted", `{"code":0}`, false, true},
		{"missing or forbidden", `{"code":90003}`, true, false},
		{"provider failure", `{"code":10002}`, false, false},
		{"missing result", `{}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			adapter := NewFunCloudMaterialAdapter("https://provider.example", "fixture-key", assetHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "/api/v2/open/material/delete", req.URL.Path)
				assert.Equal(t, "material+opaque&value", req.URL.Query().Get("materialId"))
				assert.Len(t, req.URL.Query(), 1)
				assert.Equal(t, "Bearer fixture-key", req.Header.Get("Authorization"))
				return assetJSONResponse(test.body), nil
			}))
			err := adapter.DeleteAsset(context.Background(), "material+opaque&value")
			if test.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, test.notFound, IsUpstreamNotFound(err))
			assert.Equal(t, 1, calls)
		})
	}
}
