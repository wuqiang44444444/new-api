package router

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestVideoRouterExposesOnlySelectedNorthboundContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)
	routes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	// rc.31 retires the classic OpenAI video create/read routes in favor of the
	// task plugin endpoint and the /v1/videos/:task_id/content artifact proxy.
	expected := []string{
		http.MethodPost + " /v1/videos/:video_id/remix",
		http.MethodPost + " /v1/video/generations",
		http.MethodGet + " /v1/video/generations/:task_id",
		http.MethodPost + " /api/v3/contents/generations/tasks",
		http.MethodGet + " /api/v3/contents/generations/models",
		http.MethodGet + " /api/v3/contents/generations/tasks",
		http.MethodGet + " /api/v3/contents/generations/tasks/:task_id",
		http.MethodDelete + " /api/v3/contents/generations/tasks/:task_id",
		http.MethodPost + " /kling/v1/videos/text2video",
		http.MethodPost + " /kling/v1/videos/image2video",
		http.MethodGet + " /kling/v1/videos/text2video/:task_id",
		http.MethodGet + " /kling/v1/videos/image2video/:task_id",
		http.MethodPost + " /jimeng/",
	}
	for _, route := range []string{
		http.MethodDelete + " /v1/videos/:task_id",
		http.MethodPost + " /v1/videos",
		http.MethodGet + " /v1/videos/:task_id",
		http.MethodGet + " /v1/videos/:task_id/content",
	} {
		_, exists := routes[route]
		assert.False(t, exists, route)
	}
	for _, route := range expected {
		_, exists := routes[route]
		assert.True(t, exists, route)
	}
	for route := range routes {
		assert.False(t, strings.HasPrefix(strings.SplitN(route, " ", 2)[1], "/api/v1/contents/generations/tasks"), route)
	}
}
