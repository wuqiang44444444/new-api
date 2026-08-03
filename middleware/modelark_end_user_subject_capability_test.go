package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestModelArkVideoCreateRejectsEndUserSubjectForSKUWithoutManagedAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
		c.Set("token_id", 321)
		c.Next()
	}, ModelArkVideoCreateConvert(), func(c *gin.Context) {
		t.Fatal("request should have been aborted")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"`+model.VideoSKUSeedance20Standard720P+`",
		"end_user_subject":"customer-42",
		"content":[{"type":"text","text":"make a video"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}
