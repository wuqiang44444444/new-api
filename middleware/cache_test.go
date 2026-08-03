package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheKeepsDocsManifestRevalidatable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Cache())
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name      string
		path      string
		cacheRule string
	}{
		{
			name:      "manifest",
			path:      "/docs-content/manifest.json",
			cacheRule: "no-cache, must-revalidate",
		},
		{
			name:      "manifest query",
			path:      "/docs-content/manifest.json?v=current",
			cacheRule: "no-cache, must-revalidate",
		},
		{
			name:      "versioned markdown",
			path:      "/docs-content/zh/index.md?v=current",
			cacheRule: "max-age=604800",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusNoContent, recorder.Code)
			assert.Equal(t, test.cacheRule, recorder.Header().Get("Cache-Control"))
		})
	}
}
