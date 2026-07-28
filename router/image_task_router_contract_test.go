package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRelayRouterExposesUnifiedImageCreateAndTaskQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)
	routes := make(map[string]int)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path]++
	}

	assert.Equal(t, 1, routes[http.MethodPost+" /v1/images/generations"])
	assert.Equal(t, 1, routes[http.MethodGet+" /v1/images/tasks/:task_id"])
}
