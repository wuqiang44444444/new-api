package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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
func FreezeImageTaskBilling(ctx context.Context, task *model.Task, info *relaycommon.RelayInfo, request *dto.ImageRequest) error {
	data := task.PrivateData.ImageTask
	if data.NativeRequest != nil {
		freezeImageTaskViolationFeePolicy(task)
		return freezeNativeImageBilling(ctx, task, info, request)
	}
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
	if info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeAsyncImage {
		probe.Body, err = ImageRelayBillingBody(&north, len(data.Inputs))
	} else {
		probe.Body, err = common.Marshal(north)
	}
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

// imageTaskTargetQuota restores frozen evidence without ledger or Task writes.
func imageTaskTargetQuota(ctx context.Context, task *model.Task, usage *dto.Usage) (int, *common.QuotaClamp, error) {
	data := task.PrivateData.ImageTask
	bc := task.PrivateData.BillingContext
	if data == nil || bc == nil {
		return 0, nil, errors.New("image billing snapshot is missing")
	}
	if data.FreeModel {
		zeroTaskCalculation(task, "free")
		return 0, nil, nil
	}
	if data.NativeRequest == nil && (data.ChannelType == constant.ChannelTypeGemini || data.ChannelType == constant.ChannelTypeVertexAi) {
		if err := ValidateGeminiImageUsage(data.UpstreamModel, usage); err != nil {
			return 0, nil, err
		}
	}
	billingDefaults := billingexpr.NewCalculation()
	if data.NativeRequest != nil {
		// Persist actual evidence (including absent vs zero usage). Apply the
		// native ImageHelper's billing defaults only to a calculation copy.
		nativeUsage := dto.Usage{}
		if usage != nil {
			nativeUsage = *usage
		}
		if nativeUsage.TotalTokens == 0 {
			billingDefaults.Add("default_if_zero", "token", 1, nativeUsage.TotalTokens)
			nativeUsage.TotalTokens = 1
		}
		if nativeUsage.PromptTokens == 0 {
			billingDefaults.Add("default_if_zero", "token", 1, nativeUsage.PromptTokens)
			nativeUsage.PromptTokens = 1
		}
		usage = &nativeUsage
	}
	if (bc.PerCallBilling && data.NativeRequest == nil) || usage == nil {
		retainTaskInitialCalculation(task)
		return data.HeldQuota, nil, nil
	}
	if bc.TieredSnapshot != nil {
		input := billingexpr.RequestInput{}
		if data.NativeRequest != nil && len(data.NativeRequest.BillingProbe) > 0 {
			encoded, err := restoreNativeImageBilling(ctx, task)
			if err != nil {
				return 0, nil, err
			}
			if err := common.Unmarshal([]byte(encoded), &input); err != nil {
				return 0, nil, err
			}
		}
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
		normalization := billingDefaults
		params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(snap.ExprString), normalization)
		input.RecordCalculation = true
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
		// ComputeTieredQuotaWithRequest ends with round. Replace that step so
		// normalization and the frozen contract discount precede final rounding.
		result.Calculation.Steps = result.Calculation.Steps[:len(result.Calculation.Steps)-1]
		result.Calculation.Steps = append(normalization.Steps, result.Calculation.Steps...)
		if bc.ContractFact != nil {
			result.Calculation.Add("contract_ratio", "quota", amount.String(), decimal.NewFromFloat(result.ActualQuotaBeforeGroup).Mul(decimal.NewFromFloat(snap.GroupRatio)).String(), bc.ContractFact.RatioString())
		}
		result.Calculation.Add("round", "quota", quota, amount.InexactFloat64())
		setTaskCalculation(task, result.Calculation.Finish(quota), "settlement")
		return quota, clamp, nil
	}
	if data.Price == nil {
		return 0, nil, errors.New("image price snapshot is missing")
	}
	price := *data.Price
	price.ReplaceOtherRatios(bc.OtherRatios)
	if data.NativeRequest != nil && bc.PerCallBilling && data.ImageCount > 0 && data.ImageCount <= int(dto.MaxImageN) {
		price.AddOtherRatio("n", float64(data.ImageCount))
	}
	info := &relaycommon.RelayInfo{
		BillingCalculation: billingDefaults,
		ChannelMeta:        data.BuildImageTaskChannelMeta(task.ChannelId),
		OriginModelName:    taskModelName(task), PriceData: price,
		ContractBillingFact: bc.ContractFact, StartTime: time.Unix(task.SubmitTime, 0),
		FinalPreConsumedQuota: data.HeldQuota,
	}
	// This native calculator is side-effect-free for image usage (no tool calls).
	c := &gin.Context{Request: &http.Request{Header: http.Header{}}}
	summary := calculateTextQuotaSummary(c, info, usage)
	setTaskCalculation(task, info.BillingCalculation.Finish(summary.Quota), "settlement")
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
	var target int
	var clamp *common.QuotaClamp
	var err error
	if async.TargetQuota != nil {
		target, clamp = *async.TargetQuota, async.QuotaClamp
	} else if task.Status.ShouldRefundOnTerminal() {
		target, clamp, err = frozenImageTaskViolationFee(task)
	} else {
		target, clamp, err = imageTaskTargetQuota(ctx, task, task.PrivateData.ImageTask.Usage)
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
	DeliverTaskBillingLogs(ctx, task.ID, 10)
}
