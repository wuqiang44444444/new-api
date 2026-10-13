package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Durable provider POSTs need explicit proof that a previous attempt sent no bytes.
func decideDurableTaskRetry(c *gin.Context, info *relaycommon.RelayInfo, err *taskdto.TaskError, remaining int) service.PolicyDecision {
	stop := service.PolicyDecision{Action: "stop", Reason: "durable_task_create", Source: "system"}
	if info != nil && info.ChannelMeta != nil && (info.ChannelType == constant.ChannelTypeSeedanceLink || info.ChannelType == constant.ChannelTypeMiniMaxLink) {
		return stop
	}
	if service.RequiresVideoTaskCreateAttempt(info) {
		if err == nil || err.LocalError || relaycommon.GetTaskCreateDisposition(c) != relaycommon.TaskCreateSafeToRetryBeforeCreate {
			return stop
		}
	}
	decision := decideTaskRetry(c, err, remaining)
	if decision.Action == "retry" && common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID) != 0 {
		if resetErr := service.ResetRejectedTaskCreateAttemptForRetry(c, info); resetErr != nil {
			common.SysError("release safe-to-retry task create attempt error: " + resetErr.Error())
			return stop
		}
	}
	return decision
}

func persistSubmittedTask(c *gin.Context, task *model.Task, omits ...string) error {
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	idempotencyID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskIdempotencyID))
	if attemptID != 0 {
		if err := model.RecordTaskCreateAttemptUpstreamSuccess(attemptID, task); err != nil {
			return err
		}
		return model.InsertTaskWithCreateAttempt(task, idempotencyID, attemptID)
	}
	if idempotencyID != 0 {
		if err := model.RecordTaskCreateUpstreamSuccess(idempotencyID, task); err != nil {
			return err
		}
		return model.InsertTaskWithIdempotency(task, idempotencyID)
	}
	return task.InsertWithContext(c.Request.Context(), omits...)
}

func prepareSelectedGroupBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info.RelayFormat == types.RelayFormatOpenAIImage && relay.ImageAsyncPreferRequested(c) {
		return nil
	}
	meta := &types.TokenCountMeta{}
	if info.Request != nil {
		meta = info.Request.GetTokenCountMeta()
	}
	if err := prepareCustomerContractRelayBilling(c, info, info.GetEstimatePromptTokens(), meta); err != nil {
		return err
	}
	return service.PrepareTieredBillingForSelectedGroup(c, info)
}
