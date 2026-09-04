package router

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed web/dist/placeholder.txt
var webRouterTestAssets embed.FS

func TestDocsRouteRefreshReturnsFrontendIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetWebRouter(engine, WebAssets{
		BuildFS:   webRouterTestAssets,
		IndexPage: []byte("<html><body>frontend index</body></html>"),
	}, func(c *gin.Context) {})

	request := httptest.NewRequest(http.MethodGet, "/docs/api-reference/images/generations", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/html; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	assert.Contains(t, recorder.Body.String(), "frontend index")
}
