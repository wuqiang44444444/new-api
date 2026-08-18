package service

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type TaskCreateAttemptManualRecovery struct {
	AttemptID         string `json:"attempt_id"`
	PublicTaskID      string `json:"public_task_id"`
	UpstreamTaskID    string `json:"upstream_task_id"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	Status            string `json:"status"`
	BillingHoldState  string `json:"billing_hold_state"`
}

func StageVideoTaskCreateAttemptRecovery(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	platform constant.TaskPlatform,
) error {
	if c == nil || info == nil || info.TaskRelayInfo == nil {
		return errors.New("video task recovery context is incomplete")
	}
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	if attemptID == 0 {
		return nil
	}
	now := common.GetTimestamp()
	task := model.InitTask(platform, info)
	task.CreatedAt = now
	task.UpdatedAt = now
	task.SubmitTime = now
	task.ClientProtocol = info.TaskRelayInfo.ClientProtocol
	task.Action = info.Action
	task.Quota = taskRecoveryTemplateQuota(info)
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.NodeName = common.NodeName
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      info.PriceData.ModelPrice,
		GroupRatio:      info.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:      info.PriceData.ModelRatio,
		OtherRatios:     info.PriceData.OtherRatios(),
		OriginModelName: info.OriginModelName,
		PerCallBilling:  common.StringsContains(constant.TaskPricePatches, info.OriginModelName) || info.PriceData.UsePrice,
		ContractFact:    info.ContractBillingFact,
	}
	model.AttachAsyncTaskBilling(&task.PrivateData, info, task.Quota)
	stageTaskProtocolSnapshot(c, task, info)
	return model.RecordTaskCreateAttemptRecoveryTemplate(attemptID, task)
}

func RecoverUnknownTaskCreateAttempt(
	attemptID, upstreamTaskID, upstreamRequestID string,
	providerVerified bool,
	operatorID int,
	note string,
) (*TaskCreateAttemptManualRecovery, error) {
	if !providerVerified {
		return nil, errors.New("provider verification is required before manual recovery")
	}
	internalID, err := model.PromoteTaskCreateAttemptManualSuccess(
		attemptID,
		upstreamTaskID,
		upstreamRequestID,
		operatorID,
		note,
	)
	if err != nil {
		return nil, err
	}
	task, err := model.RecoverTaskCreateAttempt(internalID)
	if err != nil {
		return nil, err
	}
	attempt, err := model.GetTaskCreateAttemptByAttemptID(attemptID)
	if err != nil {
		return nil, err
	}
	return &TaskCreateAttemptManualRecovery{
		AttemptID:         attempt.AttemptID,
		PublicTaskID:      task.TaskID,
		UpstreamTaskID:    task.PrivateData.UpstreamTaskID,
		UpstreamRequestID: task.PrivateData.UpstreamRequestID,
		Status:            string(attempt.Status),
		BillingHoldState:  string(attempt.BillingHoldState),
	}, nil
}

func taskRecoveryTemplateQuota(info *relaycommon.RelayInfo) int {
	if info == nil {
		return 0
	}
	if info.FinalPreConsumedQuota > 0 {
		return info.FinalPreConsumedQuota
	}
	return info.PriceData.Quota
}

func stageTaskProtocolSnapshot(c *gin.Context, task *model.Task, info *relaycommon.RelayInfo) {
	if task == nil || info == nil || info.TaskRelayInfo == nil {
		return
	}
	task.ClientProtocol = info.TaskRelayInfo.ClientProtocol
	task.PrivateData.VideoUpstreamProtocol = info.ChannelOtherSettings.VideoUpstreamProtocol
	profile := strings.TrimSpace(string(info.ChannelOtherSettings.VideoUpstreamProfile))
	if info.ChannelType == constant.ChannelTypeSeedanceLink {
		profile = string(info.ChannelOtherSettings.VideoUpstreamProtocol.TransportProfile())
	}
	if profile == "" {
		profile = string(dto.VideoUpstreamProfileOfficial)
	}
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(
		info.ChannelType,
		dto.VideoUpstreamProfile(profile),
	)
	request, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return
	}
	seconds := request.Seconds
	if seconds == "" && request.Duration > 0 {
		seconds = strconv.Itoa(request.Duration)
	}
	serviceTier, _ := request.Metadata["service_tier"].(string)
	if contract, ok := relaycommon.GetVideoContractRequest(c); ok &&
		contract.ModelArk != nil && contract.ModelArk.ServiceTier != nil {
		serviceTier = *contract.ModelArk.ServiceTier
	}
	task.PrivateData.ClientRequest = model.TaskClientRequestSnapshot{
		Prompt:             request.Prompt,
		Seconds:            seconds,
		Size:               request.Size,
		RemixedFromVideoID: info.OriginTaskID,
		ServiceTier:        serviceTier,
	}
}
