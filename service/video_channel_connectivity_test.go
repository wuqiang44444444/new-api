package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func officialVideoConnectivityTestChannel(baseURL, key string) *model.Channel {
	channel := &model.Channel{
		Type:    constant.ChannelTypeSeedanceLink,
		BaseURL: &baseURL,
		Key:     key,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
	})
	return channel
}

func TestCheckVideoChannelConnectivityUsesReadOnlyListEndpoint(t *testing.T) {
	var requestMethod string
	var requestPath string
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMethod = r.Method
		requestPath = r.URL.RequestURI()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	t.Cleanup(upstream.Close)

	err := CheckVideoChannelConnectivity(
		context.Background(),
		officialVideoConnectivityTestChannel(upstream.URL, "video-api-key"),
	)

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, requestMethod)
	assert.Equal(t, "/api/v3/contents/generations/tasks?page_size=1", requestPath)
	assert.Equal(t, "Bearer video-api-key", authorization)
}

func TestCheckVideoChannelConnectivityAcceptsCMCCProtocol(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":0,"items":[]}`))
	}))
	t.Cleanup(upstream.Close)
	channel := officialVideoConnectivityTestChannel(upstream.URL, "video-api-key")
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3CMCC,
	})

	err := CheckVideoChannelConnectivity(context.Background(), channel)

	require.NoError(t, err)
}

func TestCheckVideoChannelConnectivityReturnsStableErrors(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "authentication rejected",
			statusCode:  http.StatusUnauthorized,
			wantCode:    ChannelConnectivityVideoRejected,
			wantMessage: "official video API rejected the request",
		},
		{
			name:        "rate limited",
			statusCode:  http.StatusTooManyRequests,
			wantCode:    ChannelConnectivityVideoUnavailable,
			wantMessage: "official video API is unavailable",
		},
		{
			name:        "provider unavailable",
			statusCode:  http.StatusBadGateway,
			wantCode:    ChannelConnectivityVideoUnavailable,
			wantMessage: "official video API is unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(`{"message":"secret provider diagnostic"}`))
			}))
			t.Cleanup(upstream.Close)

			err := CheckVideoChannelConnectivity(
				context.Background(),
				officialVideoConnectivityTestChannel(upstream.URL, "video-api-key"),
			)

			require.Error(t, err)
			assert.Equal(t, test.wantCode, ChannelConnectivityErrorCode(err))
			assert.Equal(t, test.wantMessage, err.Error())
			assert.NotContains(t, err.Error(), "secret provider diagnostic")
		})
	}
}

func TestCheckVideoChannelConnectivityRejectsIncompleteConfiguration(t *testing.T) {
	channel := officialVideoConnectivityTestChannel("https://example.invalid", "")

	err := CheckVideoChannelConnectivity(context.Background(), channel)

	require.Error(t, err)
	assert.Equal(t, ChannelConnectivityVideoNotConfigured, ChannelConnectivityErrorCode(err))
	assert.Equal(t, "official video API credentials are not configured", err.Error())

	setting, marshalErr := common.Marshal(dto.ChannelSettings{Proxy: "ftp://invalid.example"})
	require.NoError(t, marshalErr)
	channel.Key = "video-api-key"
	channel.Setting = common.GetPointer(string(setting))
	err = CheckVideoChannelConnectivity(context.Background(), channel)
	require.Error(t, err)
	assert.Equal(t, ChannelConnectivityVideoInvalidConfig, ChannelConnectivityErrorCode(err))
}
