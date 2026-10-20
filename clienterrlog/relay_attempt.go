package clienterrlog

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// BeginNativeRelayAttempt drops observations from the previous attempt. An
// attempt rejected before transport must not inherit an earlier provider ID or
// response. Explicit business reports and the client exchange remain intact.
func BeginNativeRelayAttempt(c *gin.Context) {
	c.Set(common.UpstreamRequestIdKey, "")
	capture, ok := c.Request.Context().Value(exchangeContextKey{}).(*exchangeCapture)
	if !ok {
		return
	}
	capture.mu.Lock()
	delete(capture.bodies, "upstream_request")
	delete(capture.bodies, "upstream_response")
	capture.mu.Unlock()
}

// UpstreamResponseStatus reads the observed HTTP status before any client
// status mapping. It does not parse or copy provider response bodies.
func UpstreamResponseStatus(ctx context.Context) int {
	capture, ok := ctx.Value(exchangeContextKey{}).(*exchangeCapture)
	if !ok {
		return 0
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if response := capture.bodies["upstream_response"]; response != nil {
		return response.status
	}
	return 0
}
