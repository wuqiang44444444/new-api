package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// ImageTaskViolationFeeApplies uses the native error DTO and policy predicate.
// It performs no logging and never retains the provider response.
func ImageTaskViolationFeeApplies(body []byte, status int) bool {
	var response dto.GeneralErrorResponse
	if common.Unmarshal(body, &response) != nil {
		return false
	}
	var apiErr *types.NewAPIError
	if common.GetJsonType(response.Error) == "object" {
		if openAIError := response.TryToOpenAIError(); openAIError != nil {
			apiErr = types.WithOpenAIError(*openAIError, status)
		}
	}
	if apiErr == nil {
		apiErr = types.NewOpenAIError(errors.New(response.ToMessage()), types.ErrorCodeBadResponseStatusCode, status)
	}
	return shouldChargeViolationFee(NormalizeViolationFeeError(apiErr))
}

func freezeImageTaskViolationFeePolicy(task *model.Task) {
	settings := model_setting.GetGrokSettings()
	policy := &model.TaskImageViolationFeePolicy{}
	if settings != nil {
		policy.Enabled, policy.BaseAmount = settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount
	}
	task.PrivateData.ImageTask.ViolationFeePolicy = policy
}

func frozenImageTaskViolationFee(task *model.Task) (int, *common.QuotaClamp, error) {
	data := task.PrivateData.ImageTask
	if data == nil || !data.ViolationMarker {
		return 0, nil, nil
	}
	policy, billing := data.ViolationFeePolicy, task.PrivateData.BillingContext
	if policy == nil || billing == nil {
		return 0, nil, errors.New("image violation fee policy snapshot is missing")
	}
	if !policy.Enabled {
		return 0, nil, nil
	}
	quota, clamp := calcViolationFeeQuotaChecked(policy.BaseAmount, billing.GroupRatio)
	return quota, clamp, nil
}

// Only an actually settled positive fee is projected as a charge. A marker by
// itself (disabled policy, zero ratio, unknown result) is not billing evidence.
func appendImageTaskViolationFeeLog(other *model.LogOther, task *model.Task, chargedQuota int) {
	if !model.IsImageTask(task) || !task.Status.ShouldRefundOnTerminal() || chargedQuota <= 0 {
		return
	}
	fee, _, err := frozenImageTaskViolationFee(task)
	if err != nil || fee != chargedQuota {
		return
	}
	other.MergePublic(map[string]any{
		"violation_fee":      true,
		"violation_fee_code": string(types.ErrorCodeViolationFeeGrokCSAM),
		"fee_quota":          fee,
		"base_amount":        task.PrivateData.ImageTask.ViolationFeePolicy.BaseAmount,
		"group_ratio":        task.PrivateData.BillingContext.GroupRatio,
		"status_code":        task.PrivateData.ImageTask.FailureStatus,
	})
}
