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
	observeUpstreamResponseStatus(c.Request.Context(), 0)
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
	report, ok := ctx.Value(contextKey{}).(*requestReport)
	if !ok {
		return 0
	}
	report.mu.Lock()
	defer report.mu.Unlock()
	return report.upstreamStatus
}

// Status is transport metadata, independent of optional HTTP body diagnostics.
func observeUpstreamResponseStatus(ctx context.Context, status int) {
	report, ok := ctx.Value(contextKey{}).(*requestReport)
	if !ok {
		return
	}
	report.mu.Lock()
	report.upstreamStatus = status
	report.mu.Unlock()
}
