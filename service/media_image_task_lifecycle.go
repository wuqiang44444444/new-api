package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	mediaImageTaskQueryPath       = "/v1/media/tasks/{task_id}"
	mediaImageTaskMaxResponseSize = 1 << 20
)

type MediaImageTaskCreateSpec struct {
	UpstreamTaskID      string
	CreateRequestID     string
	QueryBaseURL        string
	Proxy               string
	AuthType            string
	AuthName            string
	AuthValueTemplate   string
	ResponseFormat      string
	RequestedImageCount uint
}

func PersistMediaImageTask(c *gin.Context, info *relaycommon.RelayInfo, spec MediaImageTaskCreateSpec) (*model.Task, error) {
	if c == nil || info == nil || info.TaskRelayInfo == nil {
		return nil, errors.New("media image task context is incomplete")
	}
	if strings.TrimSpace(spec.UpstreamTaskID) == "" {
		return nil, errors.New("upstream media image task id is required")
	}
	if spec.RequestedImageCount == 0 || spec.RequestedImageCount > dto.MaxImageN {
		return nil, fmt.Errorf("requested image count must be between 1 and %d", dto.MaxImageN)
	}
	if info.TaskRelayInfo.PublicTaskID == "" {
		info.TaskRelayInfo.PublicTaskID = model.GenerateTaskID()
	}

	now := common.GetTimestamp()
	task := model.InitTask(constant.TaskPlatformMediaImage, info)
	// InitTask also serves video protocols and may populate their generic
	// upstream snapshot when a client protocol is present. Keep image facts in
	// the media_image namespace so persisted image tasks never depend on or
	// expose video-specific fields.
	task.PrivateData.VideoUpstreamProfile = ""
	task.PrivateData.VideoUpstreamQueryBaseURL = ""
	task.PrivateData.VideoUpstreamQueryPathTemplate = ""
	task.PrivateData.VideoUpstreamProxy = ""
	task.CreatedAt = now
	task.UpdatedAt = now
	task.SubmitTime = now
	task.Status = model.TaskStatusQueued
	task.Progress = "0%"
	task.Action = constant.TaskActionImageGeneration
	task.ClientProtocol = model.TaskClientProtocolOpenAIImages
	task.Quota = info.FinalPreConsumedQuota
	task.PrivateData.Key = info.ApiKey
	task.PrivateData.UpstreamTaskID = spec.UpstreamTaskID
	task.PrivateData.UpstreamRequestID = spec.CreateRequestID
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.NodeName = common.NodeName
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      info.PriceData.ModelPrice,
		ModelRatio:      info.PriceData.ModelRatio,
		GroupRatio:      info.PriceData.GroupRatioInfo.GroupRatio,
		OtherRatios:     info.PriceData.OtherRatios(),
		OriginModelName: info.OriginModelName,
		PerCallBilling:  false,
	}
	task.PrivateData.MediaImage = &model.TaskMediaImagePrivateData{
		QueryBaseURL:        strings.TrimSpace(spec.QueryBaseURL),
		QueryPathTemplate:   mediaImageTaskQueryPath,
		Proxy:               strings.TrimSpace(spec.Proxy),
		AuthType:            strings.TrimSpace(spec.AuthType),
		AuthName:            strings.TrimSpace(spec.AuthName),
		AuthValueTemplate:   spec.AuthValueTemplate,
		ResponseFormat:      spec.ResponseFormat,
		RequestedImageCount: spec.RequestedImageCount,
		CreateRequestID:     spec.CreateRequestID,
		UsePrice:            info.PriceData.UsePrice,
		UsageBillingEnabled: info.TieredBillingSnapshot != nil,
	}
	model.AttachAsyncTaskBilling(&task.PrivateData, info, task.Quota)

	idempotencyID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskIdempotencyID))
	if err := model.RecordTaskCreateUpstreamSuccess(idempotencyID, task); err != nil {
		return nil, fmt.Errorf("record media image create outcome: %w", err)
	}
	if idempotencyID != 0 {
		info.BillingTransferredToTask = true
	}
	if err := model.InsertTaskWithIdempotency(task, idempotencyID); err != nil {
		return nil, fmt.Errorf("persist media image task: %w", err)
	}
	info.BillingTransferredToTask = true
	info.PersistedImageTask = model.ProjectOpenAIImageTask(task)

	// Log the durable pre-consumption once. Terminal differences are recorded by
	// the shared task reconciliation state machine.
	info.PriceData.Quota = task.Quota
	info.Action = constant.TaskActionImageGeneration
	LogTaskConsumption(c, info)
	return task, nil
}

func WaitMediaImageTask(ctx context.Context, taskID string, skipSleep bool) (*model.Task, error) {
	delay := time.Second
	for {
		task, exists, err := model.GetByOnlyTaskId(taskID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("persisted media image task was not found")
		}
		if task.Status.IsTerminal() {
			return task, nil
		}
		if !skipSleep {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return task, ctx.Err()
			case <-timer.C:
			}
		} else if err := ctx.Err(); err != nil {
			return task, err
		}

		task, err = PollMediaImageTaskOnce(ctx, task.TaskID)
		if err != nil {
			if ctx.Err() != nil {
				return task, ctx.Err()
			}
			logger.LogWarn(ctx, fmt.Sprintf("media image task %s poll deferred: %s", taskID, err.Error()))
		} else if task != nil && task.Status.IsTerminal() {
			return task, nil
		}
		if delay < 5*time.Second {
			delay *= 2
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
		}
	}
}

func mediaImageTaskResultURLs(result *mediaImageTaskPollResult) ([]string, error) {
	if result == nil {
		return nil, errors.New("upstream media image task has no result")
	}
	urls := make([]string, 0, len(result.URLs)+1)
	seen := make(map[string]struct{}, len(result.URLs)+1)
	add := func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("upstream media image task returned an invalid result URL")
		}
		if _, ok := seen[value]; ok {
			return nil
		}
		if len(urls) >= dto.MaxImageN {
			return fmt.Errorf("upstream media image task returned more than %d images", dto.MaxImageN)
		}
		seen[value] = struct{}{}
		urls = append(urls, value)
		return nil
	}
	for _, value := range result.URLs {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	if err := add(result.PrimaryURL); err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, errors.New("upstream media image task returned no result URL")
	}
	return urls, nil
}

func settleMediaImageTask(ctx context.Context, task *model.Task) {
	if task == nil || task.PrivateData.MediaImage == nil {
		return
	}
	media := task.PrivateData.MediaImage
	if async := task.PrivateData.AsyncBilling; async != nil && async.TieredSnapshot != nil {
		if media.Usage == nil {
			target := task.Quota
			async.Operation = "settle"
			async.Reason = "图片任务缺少可信 usage，保持安全预扣上界"
			async.TargetQuota = &target
			setTaskBillingState(task, model.TaskBillingStateSettled, "")
			if err := task.UpdateBilling(); err != nil {
				persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
			}
			return
		}
		params := BuildTieredTokenParams(media.Usage, media.Usage.UsageSemantic == dto.BillingUsageSemanticAnthropic, billingexpr.UsedVars(async.TieredSnapshot.ExprString))
		request := billingexpr.RequestInput{}
		if async.BillingProbe != nil {
			request = *async.BillingProbe
		}
		result, err := billingexpr.ComputeTieredQuotaWithRequest(async.TieredSnapshot, params, request)
		if err != nil {
			persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
			return
		}
		async.ActualTokens = media.Usage.TotalTokens
		async.Operation = "settle"
		async.Reason = fmt.Sprintf("图片 usage 表达式结算：tier=%s", result.MatchedTier)
		async.TargetQuota = &result.ActualQuotaAfterGroup
		if err := task.UpdateBilling(); err != nil {
			persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
			return
		}
		recalculateTaskQuotaWithReconcile(ctx, task, result.ActualQuotaAfterGroup, async.Reason, result.Clamp)
		return
	}

	actualCount := len(media.ResultURLs)
	if actualCount <= 0 || actualCount > dto.MaxImageN {
		return
	}
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil {
		return
	}
	if media.UsePrice {
		priceData := &types.PriceData{}
		priceData.ReplaceOtherRatios(billingContext.OtherRatios)
		priceData.AddOtherRatio("n", float64(actualCount))
		actualFloat := priceData.ApplyOtherRatiosToFloat(billingContext.ModelPrice * common.QuotaPerUnit * billingContext.GroupRatio)
		actualQuota, clamp := common.QuotaFromFloatChecked(actualFloat)
		recalculateTaskQuotaWithReconcile(ctx, task, actualQuota, fmt.Sprintf("图片实际数量结算：n=%d", actualCount), clamp)
		return
	}
	if task.PrivateData.AsyncBilling != nil {
		target := task.Quota
		task.PrivateData.AsyncBilling.Operation = "settle"
		task.PrivateData.AsyncBilling.Reason = "图片非固定价格任务保持预扣额度"
		task.PrivateData.AsyncBilling.TargetQuota = &target
		setTaskBillingState(task, model.TaskBillingStateSettled, "")
		if err := task.UpdateBilling(); err != nil {
			persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
		}
	}
}

func normalizeMediaImageUsage(usage *dto.Usage) (*dto.Usage, error) {
	if usage == nil {
		return nil, nil
	}
	maxTokens := billing_setting.MaxTaskPreConsumeTokens
	values := []int{
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		usage.InputTokens, usage.OutputTokens, usage.PromptCacheHitTokens,
		usage.PromptTokensDetails.CachedTokens,
		usage.PromptTokensDetails.CachedCreationTokens,
		usage.PromptTokensDetails.CacheWriteTokens,
		usage.PromptTokensDetails.TextTokens,
		usage.PromptTokensDetails.ImageTokens,
		usage.PromptTokensDetails.AudioTokens,
		usage.CompletionTokenDetails.TextTokens,
		usage.CompletionTokenDetails.ImageTokens,
		usage.CompletionTokenDetails.AudioTokens,
		usage.CompletionTokenDetails.ReasoningTokens,
	}
	if usage.InputTokensDetails != nil {
		values = append(values,
			usage.InputTokensDetails.CachedTokens,
			usage.InputTokensDetails.CachedCreationTokens,
			usage.InputTokensDetails.CacheWriteTokens,
			usage.InputTokensDetails.TextTokens,
			usage.InputTokensDetails.ImageTokens,
			usage.InputTokensDetails.AudioTokens,
		)
	}
	for _, value := range values {
		if value < 0 || value > maxTokens {
			return nil, fmt.Errorf("upstream media image usage must be between 0 and %d", maxTokens)
		}
	}
	promptTokens := usage.PromptTokens
	if promptTokens == 0 {
		promptTokens = usage.InputTokens
	}
	completionTokens := usage.CompletionTokens
	if completionTokens == 0 {
		completionTokens = usage.OutputTokens
	}
	promptDetails := usage.PromptTokensDetails
	if usage.InputTokensDetails != nil {
		if promptDetails.CachedTokens == 0 {
			promptDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
		}
		if promptDetails.CachedCreationTokens == 0 {
			promptDetails.CachedCreationTokens = usage.InputTokensDetails.CachedCreationTokens
		}
		if promptDetails.CacheWriteTokens == 0 {
			promptDetails.CacheWriteTokens = usage.InputTokensDetails.CacheWriteTokens
		}
		if promptDetails.TextTokens == 0 {
			promptDetails.TextTokens = usage.InputTokensDetails.TextTokens
		}
		if promptDetails.ImageTokens == 0 {
			promptDetails.ImageTokens = usage.InputTokensDetails.ImageTokens
		}
		if promptDetails.AudioTokens == 0 {
			promptDetails.AudioTokens = usage.InputTokensDetails.AudioTokens
		}
	}
	normalized := &dto.Usage{
		PromptTokens:           promptTokens,
		CompletionTokens:       completionTokens,
		TotalTokens:            usage.TotalTokens,
		InputTokens:            usage.InputTokens,
		OutputTokens:           usage.OutputTokens,
		PromptTokensDetails:    promptDetails,
		CompletionTokenDetails: usage.CompletionTokenDetails,
	}
	switch usage.UsageSemantic {
	case dto.BillingUsageSemanticOpenAI, dto.BillingUsageSemanticAnthropic, dto.BillingUsageSemanticGemini:
		normalized.UsageSemantic = usage.UsageSemantic
	}
	if normalized.TotalTokens == 0 {
		normalized.TotalTokens = normalized.PromptTokens + normalized.CompletionTokens
	}
	if normalized.TotalTokens <= 0 {
		return nil, errors.New("upstream media image usage is empty")
	}
	return normalized, nil
}

func validateMediaImageUsage(usage *dto.Usage) error {
	_, err := normalizeMediaImageUsage(usage)
	return err
}
