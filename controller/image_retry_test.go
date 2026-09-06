package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleImageDisconnectDoesNotRetryGeneration(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if !assert.NoError(t, err) {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-3.1-flash-image")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations, OriginModelName: "gemini-3.1-flash-image", Request: &dto.ImageRequest{Model: "gemini-3.1-flash-image", Prompt: "cat"}}
	apiErr := relay.ImageHelper(c, info)
	require.NotNil(t, apiErr)
	assert.False(t, shouldRetry(c, apiErr, 3), "receiving the POST then dropping the response must not allow another generation")
	assert.Equal(t, 1, posts)
}
