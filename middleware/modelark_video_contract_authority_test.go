package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestModelArkConverterDefersScalarValuesToSKUCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", ModelArkVideoCreateConvert(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v3/contents/generations/tasks",
		strings.NewReader(`{
			"model":"future-model",
			"content":[{"type":"text","text":"move"}],
			"callback_url":"https://example.com/callback",
			"duration":30,
			"resolution":"2k",
			"ratio":"2.35:1",
			"service_tier":"priority",
			"execution_expires_after":1,
			"frames":1,
			"seed":-2,
			"safety_identifier":"future-contract-value"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestModelArkPublishedSKUCapabilityRejectsUnsupportedScalars(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"model":"seedance-2.0-standard-720p","content":[{"type":"text","text":"move"}],"callback_url":"https://example.com/callback"}`,
		`{"model":"seedance-2.0-standard-720p","content":[{"type":"text","text":"move"}],"frames":1}`,
	} {
		engine := gin.New()
		engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolModelArkV3)
			c.Next()
		}, ModelArkVideoCreateConvert(), ResolveVideoSKUCapability(), func(c *gin.Context) {
			t.Fatal("unsupported scalar must not pass the published capability")
		})
		request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		engine.ServeHTTP(response, request)

		assert.Equal(t, http.StatusBadRequest, response.Code)
	}
}
