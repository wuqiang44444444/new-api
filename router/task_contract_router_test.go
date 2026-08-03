package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTaskContractOperationsExposeOnlyRootOperationalRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerTaskContractRoutes(engine.Group("/api"))
	routes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		http.MethodGet + " /api/task-contract/provider-exposures/metrics",
		http.MethodGet + " /api/task-contract/provider-exposures/incidents",
		http.MethodPost + " /api/task-contract/provider-exposures/incidents/:id/resolve",
		http.MethodGet + " /api/task-contract/attempts",
		http.MethodPost + " /api/task-contract/attempts/:attempt_id/recover",
		http.MethodPost + " /api/task-contract/attempts/:attempt_id/reject",
	} {
		_, exists := routes[route]
		assert.True(t, exists, route)
	}
}
