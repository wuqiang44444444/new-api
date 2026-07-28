package service

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// AppendUpstreamTaskTraceAdminInfo keeps blocking upstream task identifiers
// admin-only by nesting them under the existing admin_info log object.
func AppendUpstreamTaskTraceAdminInfo(ctx *gin.Context, adminInfo map[string]interface{}) {
	if adminInfo == nil {
		return
	}
	trace, ok := relaycommon.GetUpstreamTaskTrace(ctx)
	if !ok {
		return
	}
	audit := trace.AuditMap()
	if len(audit) == 0 {
		return
	}
	adminInfo["upstream_task"] = audit
}
