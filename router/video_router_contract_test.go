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

	expected := []string{
		http.MethodPost + " /v1/videos",
		http.MethodGet + " /v1/videos",
		http.MethodGet + " /v1/videos/:task_id",
		http.MethodDelete + " /v1/videos/:task_id",
		http.MethodGet + " /v1/videos/:task_id/content",
		http.MethodPost + " /v1/videos/:video_id/remix",
		http.MethodPost + " /api/v3/contents/generations/tasks",
		http.MethodGet + " /api/v3/contents/generations/tasks",
		http.MethodGet + " /api/v3/contents/generations/tasks/:task_id",
		http.MethodDelete + " /api/v3/contents/generations/tasks/:task_id",
	}
	for _, route := range expected {
		_, exists := routes[route]
		assert.True(t, exists, route)
	}
	for route := range routes {
		assert.False(t, strings.HasPrefix(strings.SplitN(route, " ", 2)[1], "/api/v1/contents/generations/tasks"), route)
	}
}
