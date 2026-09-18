package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Existing task reads/cancellation use their frozen authorization and billing
// facts. Asset proxy operations are model-keyed new calls, not durable task reads.
func contractHistoricalOperation(c *gin.Context) bool {
	path := c.Request.URL.Path
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodDelete {
		return !strings.Contains(path, "/models") && path != "/api/pricing" && !strings.HasPrefix(path, "/v1/asset")
	}
	return (strings.Contains(path, "/mj/") && strings.HasSuffix(path, "/task/list-by-condition")) || strings.HasSuffix(c.HandlerName(), ".RelayTaskFetch") || (strings.HasPrefix(path, "/v1/batches/") && strings.HasSuffix(path, "/cancel"))
}
