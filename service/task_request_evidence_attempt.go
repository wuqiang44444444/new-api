package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Storage may have waited while recovery changed the attempt. Never send or
// release funds based on the stale request-local sending state in that case.
func verifyEvidenceAttemptBeforeSend(c *gin.Context, info *relaycommon.RelayInfo) error {
	if c == nil {
		return nil
	}
	id := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	if id == 0 {
		return nil
	}
	allowed, err := model.TaskCreateAttemptAllowsEvidenceSend(id)
	if err == nil && allowed {
		return nil
	}
	common.SetContextKey(c, constant.ContextKeyTaskCreateOutcomeUnknown, true)
	if info != nil {
		info.SkipRequestRefund = true
	}
	failure := &TaskRequestEvidenceError{Cause: err}
	return failure
}
