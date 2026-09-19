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
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodDelete {
		for _, resource := range []string{"/v1/batches", "/v1/files", "/v1/videos", "/v1/video/generations", "/v1/tasks", "/v1/responses", "/api/v3/contents/generations/tasks", "/kling/v1/videos"} {
			if path == resource || strings.HasPrefix(path, resource+"/") {
				return true
			}
		}
		if strings.Contains(path, "/mj/task/") {
			return true
		}
	}
	return (strings.Contains(path, "/mj/") && strings.HasSuffix(path, "/task/list-by-condition")) || strings.HasSuffix(c.HandlerName(), ".RelayTaskFetch") || (strings.HasPrefix(path, "/v1/batches/") && strings.HasSuffix(path, "/cancel"))
}
