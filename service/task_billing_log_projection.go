package service

import (
	"strconv"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func appendTaskLogModelMapping(other *model.LogOther, task *model.Task, upstreamModel string) {
	if task != nil && (task.ClientProtocol == model.TaskClientProtocolModelArkV3 ||
		task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)) ||
		task.Platform == constant.TaskPlatform(model.MiniMaxLinkTaskPlatform())) {
		other.SetAdmin("upstream_model_name", upstreamModel)
		return
	}
	other.SetPublic("is_model_mapped", true)
	other.SetPublic("upstream_model_name", upstreamModel)
}

func taskStatementBillingMode(task *model.Task) string {
	// Failed image tasks can produce a fixed violation fee instead of an image charge.
	if bc := task.PrivateData.BillingContext; model.IsImageTask(task) &&
		task.Status == model.TaskStatusSuccess && bc != nil && !bc.PerCallBilling {
		return model.BillingReconciliationModeToken
	}
	return model.BillingReconciliationModePerCall
}
