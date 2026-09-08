package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetConnectivityWithoutDocumentedProbeDoesNotCallProvider(t *testing.T) {
	for _, protocol := range []dto.AssetUpstreamProtocol{dto.AssetUpstreamProtocolTokenSaveAssetsV1, dto.AssetUpstreamProtocolMoxingJoyCreatorV1} {
		t.Run(string(protocol), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			channel := &model.Channel{Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, BaseURL: &server.URL, Key: "fixture-secret-key"}
			channel.SetOtherSettings(dto.ChannelOtherSettings{AssetUpstreamProtocol: protocol})
			err := CheckAssetChannelConnectivity(context.Background(), channel)
			require.Error(t, err)
			assert.Equal(t, "asset_connectivity_unsupported", ChannelConnectivityErrorCode(err))
			assert.ErrorIs(t, err, ErrUnsupportedAssetOperation)
			assert.Zero(t, calls.Load())
			assert.NotContains(t, err.Error(), "fixture-secret-key")
			assert.NotContains(t, err.Error(), "configuration is invalid")
		})
	}
}
