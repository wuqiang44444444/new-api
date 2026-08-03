package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactAssetBearerToken(t *testing.T) {
	consentToken := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUV"

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "verification redirect",
			path: "/verification/real-person/" + consentToken,
			want: "/verification/real-person/:token",
		},
		{
			name: "completion route drops callback query",
			path: "/verification/real-person/complete?authorization_id=rpa_123",
			want: "/verification/real-person/complete",
		},
		{
			name: "unrelated path and query remain unchanged",
			path: "/v1/assets/asset_123?include=bindings&limit=10",
			want: "/v1/assets/asset_123?include=bindings&limit=10",
		},
		{
			name: "invalid token on sensitive route is also redacted",
			path: "/verification/real-person/not-a-valid-token",
			want: "/verification/real-person/:token",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, redactAssetBearerToken(test.path))
		})
	}
}

func TestSetUpLoggerDoesNotLogAssetBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	consentToken := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUV"

	var output bytes.Buffer
	originalWriter := gin.DefaultWriter
	gin.DefaultWriter = &output
	t.Cleanup(func() {
		gin.DefaultWriter = originalWriter
	})

	router := gin.New()
	SetUpLogger(router)
	router.GET("/verification/real-person/:token", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/verification/real-person/"+consentToken+"?locale=zh-CN", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	logLine := output.String()
	assert.NotContains(t, logLine, consentToken)
	assert.Contains(t, logLine, "/verification/real-person/:token?locale=zh-CN")
}
