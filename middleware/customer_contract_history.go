package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func jimengHistoricalOperation(c *gin.Context) bool {
	return c.Request.Method == http.MethodPost && c.FullPath() == "/jimeng/" && c.Query("Action") == "CVSync2AsyncGetResult"
}

// Existing task reads/cancellation use their frozen authorization and billing
// facts. Asset proxy operations are model-keyed new calls, not durable task reads.
func contractHistoricalOperation(c *gin.Context) bool {
	path := c.Request.URL.Path
	if jimengHistoricalOperation(c) {
		return true
	}
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodDelete {
		if strings.HasPrefix(path, "/v1/responses/") {
			return true
		}
		for _, resource := range []string{"/v1/batches", "/v1/files", "/v1/videos", "/v1/video/generations", "/v1/tasks", "/api/v3/contents/generations/tasks", "/kling/v1/videos"} {
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
