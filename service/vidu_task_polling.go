package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Provider expiry is admitted only by its frozen typed contract. Do not
// broaden the shared native polling status set when adding a Link protocol.
func isViduProviderExpiry(task *model.Task, status model.TaskStatus) bool {
	return task.HasSeedanceBillingFacts() && task.PrivateData.VideoUpstreamProtocol == dto.VideoUpstreamProtocolViduModelArkV3 && status == model.TaskStatusExpired
}

func finishViduProviderExpiry(task *model.Task, result *relaycommon.TaskInfo, now int64) bool {
	if task == nil || !isViduProviderExpiry(task, task.Status) {
		return false
	}
	if task.FinishTime == 0 {
		task.FinishTime = now
	}
	// Provider execution expiry is a trusted terminal observation, distinct
	// from the platform's unresolved-observation customer-funds deadline.
	task.Progress = "100%"
	reason := result.Reason
	if reason == "" {
		reason = "upstream execution expired"
	}
	task.FailReason = task.PublicVideoErrorMessage(reason)
	return true
}
