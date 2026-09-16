package relay

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// 显式图片异步受理（§3.7/§3.8）。同一对北向入口以 Prefer: respond-async
// 选择本路径；受理事务原子占用容量、写入可执行 Task 并完成幂等绑定，
// 钱包与令牌额度同时提交，失败不留下半完成的受理事实。

// ImageAsyncPreferRequested is used by the controller to skip the generic
// request-scoped pre-consume only when a platform task will own the funds.
func ImageAsyncPreferRequested(c *gin.Context) bool {
	return service.ImageAsyncExecutionRequested(c)
}

func imageAsyncHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	native := info.ChannelType == constant.ChannelTypeOpenAI || info.ChannelType == constant.ChannelTypeAzure
	if imageReq.Stream != nil && *imageReq.Stream && !native {
		// P14：统一图片族 Prefer: respond-async 与 stream=true 互斥，在受理、
		// 预扣、调用上游前报参数冲突。原生 OpenAI／Azure 走 Task 优先：
		// stream=true 由冻结请求原样保留，后台接收上游流式结果。
		return types.NewErrorWithStatusCode(errors.New("stream=true cannot be combined with Prefer: respond-async"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	// 评审 S3：异步路径必须在族判断与冻结快照之前完成管理员模型映射，
	// 与同步路径（ModelMappedHelper 在 ImageHelper 内）保持同一语义；
	// 否则客户别名（如 nano-banana-2-gemini）会被当成 Provider 模型拒判。
	mappedRequest, err := mapImageAsyncRequest(c, info, imageReq)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	var contract *service.ImageContract
	var apiErr *types.NewAPIError
	if native {
		// Native validation already ran in the controller. Do not impose the
		// unified image contract (notably its mask rejection) on native edits.
		operation := service.ImageOperationGenerations
		if info.RelayMode == relayconstant.RelayModeImagesEdits {
			operation = service.ImageOperationEdits
		}
		n := uint(1)
		if mappedRequest.N != nil {
			n = *mappedRequest.N
		}
		contract = &service.ImageContract{Operation: operation, N: n, Prompt: mappedRequest.Prompt, Size: mappedRequest.Size, ResponseFormat: mappedRequest.ResponseFormat}
	} else if info.ApiType == constant.APITypeAsyncImage {
		contract, apiErr = service.ParseImageRelayContract(c, info, mappedRequest, info.ChannelOtherSettings.ImageUpstreamProtocol)
	} else {
		contract, apiErr = service.ParseImageContract(c, info, mappedRequest)
	}
	if apiErr != nil {
		return apiErr
	}
	if apiErr := validateImageAsyncFamilyContract(c, info, mappedRequest); apiErr != nil {
		return apiErr
	}

	config := system_setting.LoadImageTaskConfig()
	storeCtx, storeErr := service.WithImageObjectStore(c.Request.Context())
	if storeErr == nil {
		storeErr = service.CheckImageObjectStoreReady(storeCtx)
	}
	if storeErr != nil {
		// §3.10：存储不可用时拒绝新异步受理，不预扣、不发送。
		return types.NewErrorWithStatusCode(errors.New("async image execution requires the platform artifact store"), types.ErrorCodeInvalidRequest, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	c.Request = c.Request.WithContext(storeCtx)

	userID := info.UserId
	appID := 0
	if c != nil {
		appID = c.GetInt("token_id")
	}
	globalScope := model.ImageTaskAdmissionScopeGlobal()
	appScope := model.ImageTaskAdmissionScopeApp(userID, appID)

	quota := info.PriceData.QuotaToPreConsume
	if info.PriceData.FreeModel {
		quota = 0
	}

	// 评审 S6：taskID 先于输入落位生成，输入/结果对象键均为确定性键
	// （images/tasks/{taskID}/input-N|result-N），崩溃后可按键补登记/续传。
	taskID := model.GenerateTaskID()
	var task *model.Task
	if native {
		frozen, freezeErr := freezeNativeImageRequest(c, info, taskID, mappedRequest)
		if freezeErr != nil {
			return types.NewErrorWithStatusCode(errors.New("native image request could not be prepared for async execution"), types.ErrorCodeConvertRequestFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		}
		task = buildImageTask(taskID, c, info, contract, nil, int64(quota), config)
		data := task.PrivateData.ImageTask
		data.NativeRequest = frozen
		// Connection/authentication live only in the encrypted snapshot.
		data.ChannelKey, data.ChannelBaseUrl, data.ChannelProxy = "", "", ""
		data.ChannelSettings = dto.ChannelSettings{}
		data.Prompt = ""
	} else {
		headersCiphertext, err := freezeImageTaskHeaders(taskID, c, info)
		if err != nil {
			return types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
		inputRefs, apiErr := stageImageTaskInputs(c, taskID, contract)
		if apiErr != nil {
			return apiErr
		}
		task = buildImageTask(taskID, c, info, contract, inputRefs, int64(quota), config)
		task.PrivateData.ImageTask.HeadersCiphertext = headersCiphertext
	}
	// 挂接持久化计费状态机（billing_state 投影 + 幂等结算/退款与补偿扫描）。
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{
		ClientProtocol: model.TaskClientProtocolImageOpenAIV1,
		AppID:          appID,
	}
	model.AttachAsyncTaskBilling(&task.PrivateData, info, int(quota))
	if err := service.FreezeImageTaskBilling(c.Request.Context(), task, info, mappedRequest); err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
	}

	idempotencyID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskIdempotencyID))
	if err := model.InsertImageTask(model.ImageTaskInsertParams{
		Task:              task,
		FundingPreference: info.UserSetting.BillingPreference,
		IdempotencyID:     idempotencyID,
		GlobalScope:       globalScope,
		GlobalLimit:       config.MaxWaiting,
		AppScope:          appScope,
		AppLimit:          config.MaxPerApp,
	}); err != nil {
		return imageAdmissionError(err)
	}

	info.SkipRequestRefund = true
	createdAt := task.SubmitTime
	queryPath := fmt.Sprintf("/v1/tasks/%s", taskID)
	c.Header("Location", queryPath)
	c.JSON(http.StatusAccepted, gin.H{
		"created":   createdAt,
		"id":        taskID,
		"object":    "image_task",
		"status":    "queued",
		"query_url": queryPath,
	})
	return nil
}

// mapImageAsyncRequest 复制请求并应用管理员模型映射（评审 S3）。同步与
// 异步路径共用 ModelMappedHelper，保证同一客户别名得到同一 Provider 模型。
func mapImageAsyncRequest(c *gin.Context, info *relaycommon.RelayInfo, imageReq *dto.ImageRequest) (*dto.ImageRequest, error) {
	mapped, err := common.DeepCopy(imageReq)
	if err != nil {
		return nil, fmt.Errorf("failed to copy request to ImageRequest: %w", err)
	}
	if err := helper.ModelMappedHelper(c, info, mapped); err != nil {
		return nil, err
	}
	return mapped, nil
}

// validateImageAsyncFamilyContract runs the family matrix so acceptance can
// fail before capacity, storage, or funds are touched.
func validateImageAsyncFamilyContract(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest) *types.NewAPIError {
	switch info.ApiType {
	case constant.APITypeGemini, constant.APITypeVertexAi:
		if !gemini.SupportsGenerateContentImage(info.UpstreamModelName) {
			return types.NewErrorWithStatusCode(errors.New("model does not support the image contract"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		// Async delivery belongs to the platform, not the Google converter's
		// synchronous URL hosting check. The northbound format was validated above.
		providerRequest := *request
		providerRequest.ResponseFormat = "b64_json"
		if _, apiErr := gemini.ParseGeminiImageContract(c, info, &providerRequest); apiErr != nil {
			return apiErr
		}
	case constant.APITypeAsyncImage:
		// Northbound delivery is independent of the Provider's URL-only result.
		providerRequest, err := common.DeepCopy(request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		providerRequest.ResponseFormat = "url"
		adaptor := GetAdaptor(info.ApiType)
		adaptor.Init(info)
		if _, err := adaptor.ConvertImageRequest(c, info, *providerRequest); err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
	}
	return nil
}

func buildImageTask(taskID string, c *gin.Context, info *relaycommon.RelayInfo, contract *service.ImageContract, inputs []model.TaskImageInputRef, quota int64, config system_setting.ImageTaskConfig) *model.Task {
	appID := 0
	if c != nil {
		appID = c.GetInt("token_id")
	}
	execution := &model.TaskImageExecutionData{
		Operation:       string(contract.Operation),
		Prompt:          contract.Prompt,
		Size:            contract.Size,
		ResponseFormat:  contract.ResponseFormat,
		N:               contract.N,
		Inputs:          inputs,
		HeldQuota:       int(quota),
		FreeModel:       info.PriceData.FreeModel,
		ChannelType:     info.ChannelType,
		ChannelBaseUrl:  info.ChannelBaseUrl,
		ChannelKey:      info.ApiKey,
		ChannelProxy:    info.ChannelSetting.Proxy,
		ChannelSettings: info.ChannelSetting,
		ChannelOther:    info.ChannelOtherSettings,
		UpstreamModel:   info.UpstreamModelName,
		ApiVersion:      info.ApiVersion,
		QueueDeadlineAt: model.ImageTaskQueueDeadline(config.QueueSeconds),
	}
	task := model.InitTask(model.ImageTaskPlatform(info.ChannelType), info)
	task.TaskID = taskID
	task.ClientProtocol = model.TaskClientProtocolImageOpenAIV1
	if contract.Operation == service.ImageOperationEdits {
		task.Action = model.TaskActionImageEdit
	} else {
		task.Action = model.TaskActionImageGeneration
	}
	task.Status = model.TaskStatusQueued
	task.Quota = int(quota)
	task.Progress = "0%"
	task.AppID = appID
	task.PrivateData.ImageTask = execution
	task.PrivateData.AppID = appID
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      info.PriceData.ModelPrice,
		GroupRatio:      info.PriceData.GroupRatioInfo.GroupRatio,
		OriginModelName: info.OriginModelName,
		PerCallBilling:  info.PriceData.UsePrice,
		OtherRatios:     info.PriceData.OtherRatios(),
		TieredSnapshot:  info.TieredBillingSnapshot,
		ContractFact:    info.ContractBillingFact,
	}
	// 评审 S2：冻结令牌与资金来源事实，终态结算/退款走统一原子计费机。
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.BillingSource = service.BillingSourceWallet
	return task
}

func stageImageTaskInputs(c *gin.Context, taskID string, contract *service.ImageContract) ([]model.TaskImageInputRef, *types.NewAPIError) {
	refs := make([]model.TaskImageInputRef, 0, len(contract.Images))
	for index, image := range contract.Images {
		if image.IsURL() {
			refs = append(refs, model.TaskImageInputRef{URL: image.URL, MimeType: image.MimeType})
			continue
		}
		objectKey, err := service.BuildImageTaskObjectKey(taskID, fmt.Sprintf("input-%d", index))
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		if _, err := service.PutImageObject(c.Request.Context(), objectKey, image.MimeType, image.Data); err != nil {
			logger.LogWarn(c, "async image input staging failed: "+err.Error())
			return nil, types.NewErrorWithStatusCode(errors.New("failed to store input image"), types.ErrorCodeBadResponseBody, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		}
		refs = append(refs, model.TaskImageInputRef{ObjectKey: objectKey, MimeType: image.MimeType})
	}
	return refs, nil
}

func imageAdmissionError(err error) *types.NewAPIError {
	if errors.Is(err, model.ErrTaskAttemptInsufficientQuota) || errors.Is(err, model.ErrTaskAttemptSubscriptionUnavailable) {
		return types.NewErrorWithStatusCode(errors.New("insufficient funding or token quota"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	if model.IsImageSlotLimitError(err) {
		if model.IsImageSlotAppLimit(err) {
			// 应用未完成任务超限：429，均不受理、不预扣、不发送。
			return types.NewErrorWithStatusCode(errors.New("too many unfinished image tasks for this application"), types.ErrorCodeInvalidRequest, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
		}
		// 全局排队容量耗尽：503，附 Retry-After 提示（不保证届时有空位）。
		apiErr := types.NewErrorWithStatusCode(errors.New("image task queue is full"), types.ErrorCodeInvalidRequest, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
		return apiErr
	}
	return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
}
