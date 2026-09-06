package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// FreezeImageTaskBilling preserves the same request/price evidence used by native
// billing. Media lives in OSS, never in the billing probe; headers are encrypted.
func FreezeImageTaskBilling(task *model.Task, info *relaycommon.RelayInfo, request *dto.ImageRequest) error {
	data := task.PrivateData.ImageTask
	parameters, err := common.DeepCopy(request)
	if err != nil {
		return err
	}
	parameters.Image, parameters.Images, parameters.Mask = nil, nil, nil
	data.Parameters = parameters
	price := info.PriceData
	data.Price = &price
	task.PrivateData.BillingContext.ModelRatio = price.ModelRatio
	if info.TieredBillingSnapshot == nil {
		return nil
	}
	probe := billingexpr.RequestInput{}
	if info.BillingRequestInput != nil {
		probe = *info.BillingRequestInput
	}
	// Keep the customer model in param("model"), not its southbound alias.
	north := *parameters
	north.Model = info.OriginModelName
	probe.Body, err = common.Marshal(north)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(probe)
	if err != nil {
		return err
	}
	data.BillingRequestCiphertext, err = common.EncryptShortLivedSecretForScope("image-billing:"+task.TaskID, string(encoded))
	// The shared video probe deliberately has different semantics. Images restore
	// their encrypted request at their own boundary instead of storing it twice.
	task.PrivateData.AsyncBilling.BillingProbe = nil
	return err
}

// imageTaskTargetQuota is pure: no wallet, Token, Task or logging writes.
func imageTaskTargetQuota(task *model.Task, usage *dto.Usage) (int, *common.QuotaClamp, error) {
	data := task.PrivateData.ImageTask
	bc := task.PrivateData.BillingContext
	if data == nil || bc == nil {
		return 0, nil, errors.New("image billing snapshot is missing")
	}
	if data.FreeModel {
		return 0, nil, nil
	}
	if bc.PerCallBilling || usage == nil {
		return data.HeldQuota, nil, nil
	}
	if bc.TieredSnapshot != nil {
		input := billingexpr.RequestInput{}
		if data.BillingRequestCiphertext != "" {
			encoded, err := common.DecryptShortLivedSecretForScope("image-billing:"+task.TaskID, data.BillingRequestCiphertext)
			if err != nil {
				return 0, nil, errors.New("image billing request could not be restored")
			}
			if err := common.Unmarshal([]byte(encoded), &input); err != nil {
				return 0, nil, err
			}
		}
		snap := bc.TieredSnapshot
		params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(snap.ExprString))
		result, err := billingexpr.ComputeTieredQuotaWithRequest(snap, params, input)
		if err != nil {
			return 0, nil, err
		}
		amount, err := ApplyCustomerContractRatio(decimal.NewFromFloat(result.ActualQuotaBeforeGroup).
			Mul(decimal.NewFromFloat(snap.GroupRatio)), bc.ContractFact)
		if err != nil {
			return 0, nil, err
		}
		quota, clamp := common.QuotaRoundChecked(amount.InexactFloat64())
		return quota, clamp, nil
	}
	if data.Price == nil {
		return 0, nil, errors.New("image price snapshot is missing")
	}
	price := *data.Price
	price.ReplaceOtherRatios(bc.OtherRatios)
	info := &relaycommon.RelayInfo{
		ChannelMeta:     data.BuildImageTaskChannelMeta(task.ChannelId),
		OriginModelName: taskModelName(task), PriceData: price,
		ContractBillingFact: bc.ContractFact, StartTime: time.Unix(task.SubmitTime, 0),
		FinalPreConsumedQuota: data.HeldQuota,
	}
	// This native calculator is side-effect-free for image usage (no tool calls).
	c := &gin.Context{Request: &http.Request{Header: http.Header{}}}
	summary := calculateTextQuotaSummary(c, info, usage)
	return summary.Quota, info.QuotaClamp, nil
}

func settleImageTaskBilling(ctx context.Context, task *model.Task) {
	if !model.IsImageTask(task) || (task.Status != model.TaskStatusSuccess && !task.Status.ShouldRefundOnTerminal()) {
		return
	}
	async := task.PrivateData.AsyncBilling
	if async == nil || async.State == model.TaskBillingStateSettled {
		return
	}
	target, clamp, err := imageTaskTargetQuota(task, task.PrivateData.ImageTask.Usage)
	if task.Status.ShouldRefundOnTerminal() {
		target, clamp, err = 0, nil, nil
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("image task %s billing evidence could not be evaluated", task.TaskID))
		return // persisted usage remains pending, never settle a fabricated fallback
	}
	async.TargetQuota = &target
	async.QuotaClamp = clamp
	applied, _, err := model.ApplyTaskBillingTarget(task, target)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("image task %s atomic settlement failed", task.TaskID))
		return // the same frozen target is rebuilt by the shared reconcile scan
	}
	if !applied {
		return
	}
	other := taskBillingOther(task)
	other.SetPublic("task_id", task.TaskID)
	other.SetPublic("image_count", task.PrivateData.ImageTask.ImageCount)
	attachQuotaSaturationToOther(other, clamp)
	completionTokens := 0
	if usage := task.PrivateData.ImageTask.Usage; usage != nil {
		completionTokens = usage.CompletionTokens
		other.SetPublic("prompt_tokens", usage.PromptTokens)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: task.UserId, LogType: model.LogTypeConsume, ChannelId: task.ChannelId,
		ModelName: taskModelName(task), Quota: target, TokenId: task.PrivateData.TokenId,
		Group: task.Group, Other: other, CompletionTokens: completionTokens,
	})
}
