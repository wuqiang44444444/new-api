package assets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginAssetFailuresPreserveSafeDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   HTTPDoer
		want string
	}{
		{"transport", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("fixture-secret: %w", context.DeadlineExceeded)
		}), "stage=wait_response class=timeout"},
		{"response body", assetHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(iotest.ErrReader(fmt.Errorf("fixture-secret")))}, nil
		}), "stage=decode_response class=invalid_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := publishedAssetFixture(t, dto.AssetUpstreamProtocolTokenSaveAssetsV1, "https://provider.example", "fixture-key", tc.do)
			_, err := adapter.GetAsset(context.Background(), "asset-one")
			require.Error(t, err)
			diagnostic, ok := SafeUpstreamDiagnostic(err)
			require.True(t, ok)
			assert.Equal(t, tc.want, diagnostic)
			assert.NotContains(t, err.Error(), "fixture-secret")
		})
	}
}
