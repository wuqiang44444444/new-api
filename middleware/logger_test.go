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
	receiptToken := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKL"

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "consent page",
			path: "/consent/real-person/" + consentToken,
			want: "/consent/real-person/:token",
		},
		{
			name: "receipt page preserves query",
			path: "/consent/real-person/receipt/" + receiptToken + "?locale=zh-CN",
			want: "/consent/real-person/receipt/:receipt_token?locale=zh-CN",
		},
		{
			name: "accept consent",
			path: "/api/real-person-consents/" + consentToken + "/accept",
			want: "/api/real-person-consents/:token/accept",
		},
		{
			name: "reject consent",
			path: "/api/real-person-consents/" + consentToken + "/reject",
			want: "/api/real-person-consents/:token/reject",
		},
		{
			name: "revoke receipt",
			path: "/api/real-person-consents/receipt/" + receiptToken + "/revoke",
			want: "/api/real-person-consents/receipt/:receipt_token/revoke",
		},
		{
			name: "completion route drops callback query",
			path: "/consent/real-person/complete?authorization_id=rpa_123",
			want: "/consent/real-person/complete",
		},
		{
			name: "unrelated path and query remain unchanged",
			path: "/v1/assets/asset_123?include=bindings&limit=10",
			want: "/v1/assets/asset_123?include=bindings&limit=10",
		},
		{
			name: "invalid token on sensitive route is also redacted",
			path: "/consent/real-person/not-a-valid-token",
			want: "/consent/real-person/:token",
		},
		{
			name: "redirect candidate with trailing slash is redacted",
			path: "/api/real-person-consents/" + consentToken + "/accept/?source=email",
			want: "/api/real-person-consents/:token/accept/?source=email",
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
	router.GET("/consent/real-person/:token", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/consent/real-person/"+consentToken+"?locale=zh-CN", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	logLine := output.String()
	assert.NotContains(t, logLine, consentToken)
	assert.Contains(t, logLine, "/consent/real-person/:token?locale=zh-CN")
}
