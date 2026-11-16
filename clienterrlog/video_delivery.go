package clienterrlog

import (
	"context"
	"net/http"
)

// MarkVideoDeliveryWaiting records a normal Link delivery wait in the request
// diagnostic carrier. The recorder still checks the final route and status.
func MarkVideoDeliveryWaiting(ctx context.Context) {
	report, ok := ctx.Value(contextKey{}).(*requestReport)
	if !ok {
		return
	}
	report.mu.Lock()
	report.videoDeliveryWaiting = true
	report.mu.Unlock()
}

func isVideoDeliveryWait(method, route string, status int) bool {
	return status == http.StatusConflict && route == "/v1/videos/:task_id/content" &&
		(method == http.MethodGet || method == http.MethodHead)
}
