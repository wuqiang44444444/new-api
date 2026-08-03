package service

import "github.com/QuantumNous/new-api/model"

const (
	providerContractFailureReason   = "provider_contract_failure"
	upstreamOutcomeUnresolvedReason = "upstream_outcome_unresolved"
)

func taskProviderExposure(task *model.Task, reason string) *model.ProviderCostExposure {
	if task == nil {
		return nil
	}
	return &model.ProviderCostExposure{
		SourceKind:             model.ProviderCostExposureSourceTask,
		SourceID:               task.TaskID,
		Reason:                 reason,
		UserID:                 task.UserId,
		ChannelID:              task.ChannelId,
		PublicModel:            task.Properties.OriginModelName,
		UpstreamProfile:        string(task.PrivateData.VideoUpstreamProfile),
		LinkImplementationID:   task.PrivateData.LinkImplementationID,
		LinkImplementationVer:  task.PrivateData.LinkImplementationVersion,
		LinkImplementationHash: task.PrivateData.LinkImplementationHash,
	}
}

func taskTerminalBillingReason(task *model.Task, fallback string) string {
	if task == nil {
		return fallback
	}
	switch task.Status {
	case model.TaskStatusProviderContractFailure:
		return providerContractFailureReason
	case model.TaskStatusExpired:
		if fallback == "" {
			return upstreamOutcomeUnresolvedReason
		}
	}
	return fallback
}
