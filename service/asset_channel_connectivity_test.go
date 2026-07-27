package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proxyAssetConnectivityTestChannel(baseURL string, profile dto.AssetUpstreamProfile) *model.Channel {
	channel := &model.Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		BaseURL: &baseURL,
		Key:     "channel-key",
	}
	videoProfile := dto.VideoUpstreamProfileThirdPartyRelay
	if profile == dto.AssetUpstreamProfileArk {
		videoProfile = dto.VideoUpstreamProfileThirdPartyReverseProxy
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:  videoProfile,
		AssetUpstreamProfile:  profile,
		AssetMinURLTTLSeconds: 3600,
	})
	return channel
}

func TestCheckAssetChannelConnectivityCoversProxyProfiles(t *testing.T) {
	tests := []struct {
		name     string
		profile  dto.AssetUpstreamProfile
		wantPath string
	}{
		{name: "channel 27 relay protocol", profile: dto.AssetUpstreamProfileRelay, wantPath: "/assets/list"},
		{name: "channel 28 ark protocol", profile: dto.AssetUpstreamProfileArk, wantPath: "/v1/ark/assets/list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestedPath string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPath = r.URL.Path
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "Bearer channel-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(upstream.Close)

			err := CheckAssetChannelConnectivity(
				context.Background(),
				proxyAssetConnectivityTestChannel(upstream.URL, test.profile),
			)

			require.NoError(t, err)
			assert.Equal(t, test.wantPath, requestedPath)
		})
	}
}

func TestCheckAssetChannelConnectivityReturnsSafeProxyErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"private upstream diagnostic"}`))
	}))
	t.Cleanup(upstream.Close)

	err := CheckAssetChannelConnectivity(
		context.Background(),
		proxyAssetConnectivityTestChannel(upstream.URL, dto.AssetUpstreamProfileRelay),
	)

	require.Error(t, err)
	assert.Equal(t, ChannelConnectivityAssetProxyRejected, ChannelConnectivityErrorCode(err))
	assert.Equal(t, "asset upstream rejected the request", err.Error())
	assert.NotContains(t, err.Error(), "private upstream diagnostic")
}
