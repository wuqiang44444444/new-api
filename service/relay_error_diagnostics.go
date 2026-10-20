package service

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// AttachNativeRelayErrorDiagnostics runs only at the final error exit. Attaching
// on every failed attempt would freeze the first channel through Attach's
// first-writer precedence while the provider ID belongs to the last attempt.
func AttachNativeRelayErrorDiagnostics(c *gin.Context, err *types.NewAPIError) {
	if err == nil {
		return
	}
	report := clienterrlog.Report{
		Model:     c.GetString("original_model"),
		ChannelID: common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		Stage:     "relay",
		Reason:    "local_error",
	}
	status := clienterrlog.UpstreamResponseStatus(c.Request.Context())
	switch {
	case err.GetErrorCode() == types.ErrorCodeGetChannelFailed:
		// A retry can fail selection while Gin still carries the previous
		// attempt's channel. That channel did not cause this final rejection.
		report.Stage, report.Reason, report.ChannelID = "channel_selection", "channel_unavailable", 0
	case err.GetErrorCode() == types.ErrorCodeDoRequestFailed:
		report.Stage, report.Reason = "upstream_request", "upstream_transport_error"
	case types.IsChannelError(err):
		report.Stage, report.Reason = "channel_selection", "channel_error"
	case err.GetErrorType() != types.ErrorTypeNewAPIError && status >= http.StatusBadRequest:
		report.Stage, report.Reason = "upstream_response", "upstream_http_error"
		report.Detail = map[string]string{"upstream_status": strconv.Itoa(status)}
		if status == http.StatusTooManyRequests {
			report.Reason = "upstream_rate_limited"
		}
	}
	clienterrlog.Attach(c.Request.Context(), report)
}
