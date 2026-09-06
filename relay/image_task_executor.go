package relay

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/asyncimage"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/moxingimage"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 图片任务 Provider 执行器（由 main 注入 service.ImageTaskExecuteFunc）。
// 只做冻结事实驱动的南向调用与结果归一，不写任何 Task 状态；发送许可、
// 终态与结算由 service worker 持有（R7：不因当前配置重选渠道）。
// 请求构建复用各 adaptor 的 ConvertImageRequest，合同校验因此与同步路径
// 完全一致。

// maxImageResultBytes 单张结果图片（或回读输入）的字节上限。
const maxImageResultBytes = 50 * 1024 * 1024

// ExecuteImageTask runs the frozen image task against its provider.
func ExecuteImageTask(ctx context.Context, task *model.Task) service.ImageTaskExecution {
	ctx, err := service.WithImageObjectStore(ctx)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "storage_unavailable"}
	}
	data := task.PrivateData.ImageTask
	if data == nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "execution_data_missing"}
	}
	headless := newHeadlessGinContext(ctx)
	info := buildFrozenImageRelayInfo(task, data)

	request, err := rebuildImageRequest(ctx, data)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "request_build_failed"}
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "unsupported_channel_type"}
	}
	adaptor.Init(info)
	if info.ApiType == constant.APITypeAsyncImage {
		request.ResponseFormat = "url"
	}
	converted, err := adaptor.ConvertImageRequest(headless, info, *request)
	if err != nil {
		return executorFailureFromError(err)
	}
	body, err := common.Marshal(converted)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "request_build_failed"}
	}

	switch info.ApiType {
	case constant.APITypeGemini, constant.APITypeVertexAi:
		return executeGeminiImageTask(ctx, headless, task, info, body)
	case constant.APITypeAsyncImage:
		return executeImageRelayTask(ctx, task, info, body)
	default:
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "unsupported_channel_type"}
	}
}

func buildFrozenImageRelayInfo(task *model.Task, data *model.TaskImageExecutionData) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		ChannelMeta:     data.BuildImageTaskChannelMeta(task.ChannelId),
		OriginModelName: task.Properties.OriginModelName,
		UserId:          task.UserId,
		UsingGroup:      task.Group,
		StartTime:       time.Now(),
	}
	if info.OriginModelName == "" {
		info.OriginModelName = data.UpstreamModel
	}
	info.ApiType, _ = common.ChannelType2APIType(data.ChannelType)
	if data.Operation == string(service.ImageOperationEdits) {
		info.RelayMode = relayconstant.RelayModeImagesEdits
	} else {
		info.RelayMode = relayconstant.RelayModeImagesGenerations
	}
	return info
}

// rebuildImageRequest reconstructs the northbound DTO from frozen facts.
// 二进制输入以 data URL 形式回填，仅在当次 Provider 调用内存在。
func rebuildImageRequest(ctx context.Context, data *model.TaskImageExecutionData) (*dto.ImageRequest, error) {
	if data.Parameters == nil {
		return nil, errors.New("image parameters snapshot is missing")
	}
	request, err := common.DeepCopy(data.Parameters)
	if err != nil {
		return nil, err
	}
	request.Model = data.UpstreamModel
	request.ResponseFormat = data.ResponseFormat
	if request.ResponseFormat == "" {
		request.ResponseFormat = "url"
	}
	n := data.N
	if n == 0 {
		n = 1
	}
	request.N = &n

	if len(data.Inputs) > 0 {
		items := make([]string, 0, len(data.Inputs))
		for _, input := range data.Inputs {
			if input.IsURL() {
				items = append(items, input.URL)
				continue
			}
			objectBytes, err := service.FetchImageObjectBytes(ctx, input.ObjectKey)
			if err != nil {
				return nil, err
			}
			items = append(items, fmt.Sprintf("data:%s;base64,%s",
				input.MimeType, base64.StdEncoding.EncodeToString(objectBytes)))
		}
		encoded, err := common.Marshal(items)
		if err != nil {
			return nil, err
		}
		request.Images = encoded
	}
	return request, nil
}

func executeGeminiImageTask(ctx context.Context, c *gin.Context, task *model.Task, info *relaycommon.RelayInfo, body []byte) service.ImageTaskExecution {
	headers, err := restoreImageTaskHeaders(task)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "headers_snapshot_unavailable"}
	}
	responseBody, apiErr := postFrozenImageRequest(ctx, c, info, body, headers)
	if apiErr != nil {
		return executorTransportOutcome(apiErr)
	}
	results, usage, apiErr := gemini.ParseGenerateContentImageResponseBody(c, info, responseBody)
	if apiErr != nil {
		if apiErr.GetErrorCode() == types.ErrorCodePromptBlocked {
			return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "provider_rejected"}
		}
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "result_parse_failed"}
	}
	artifacts, uploadErr := storeImageResults(ctx, task, len(results), usage, func(index int) ([]byte, string, error) {
		return results[index].Data, results[index].MimeType, nil
	})
	if uploadErr != nil {
		// 生成成功而保存失败是交付异常：保持待核实，不判生成失败（§3.9）。
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "result_store_failed"}
	}
	return service.ImageTaskExecution{
		Outcome: service.ImageTaskOutcomeSuccess,
		Images:  artifacts,
		Usage:   usage,
	}
}

func executeImageRelayTask(ctx context.Context, task *model.Task, info *relaycommon.RelayInfo, body []byte) service.ImageTaskExecution {
	headers, err := restoreImageTaskHeaders(task)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "headers_snapshot_unavailable"}
	}
	var urls []string
	var providerTaskID string
	var apiErr *types.NewAPIError
	switch info.ChannelOtherSettings.ImageUpstreamProtocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		providerTaskID, urls, apiErr = asyncimage.HeadlessCreateAndPoll(ctx, info, headers, bytes.NewReader(body), func(trustedTaskID string) {
			// 评审 S6：可信上游任务 ID 取得即持久化（仅内存回调的落库通道，
			// worker 侧同时兜底消费返回值）。
			if _, err := model.MarkImageTaskProviderTaskID(task, trustedTaskID); err != nil {
				common.SysError("persist image task provider id failed: " + err.Error())
			}
		})
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		urls, apiErr = moxingimage.HeadlessGenerate(ctx, info, headers, bytes.NewReader(body))
	default:
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "image_upstream_protocol_missing"}
	}
	if apiErr != nil {
		if providerTaskID != "" {
			// A query error is not evidence that the accepted generation failed.
			return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, ProviderTaskID: providerTaskID, FailureCode: "poll_inconclusive"}
		}
		return executorTransportOutcome(apiErr)
	}
	if len(urls) == 0 {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, ProviderTaskID: providerTaskID, FailureCode: "empty_result"}
	}

	// URL 结果下载后保存到私有 OSS（异步交付按 300 秒签名续签，§5）。
	artifacts, uploadErr := storeImageResults(ctx, task, len(urls), nil, func(index int) ([]byte, string, error) {
		return downloadImageURL(ctx, urls[index])
	})
	if uploadErr != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "result_store_failed"}
	}
	return service.ImageTaskExecution{
		Outcome:        service.ImageTaskOutcomeSuccess,
		ProviderTaskID: providerTaskID,
		Images:         artifacts,
	}
}

// postFrozenImageRequest sends the marshaled body through the adaptor's URL
// and header logic with a synthetic request context.
func postFrozenImageRequest(ctx context.Context, c *gin.Context, info *relaycommon.RelayInfo, body []byte, headers map[string]string) ([]byte, *types.NewAPIError) {
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, types.NewErrorWithStatusCode(errors.New("adaptor unavailable"), types.ErrorCodeInvalidApiType, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	requestURL, err := adaptor.GetRequestURL(info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	req.ContentLength = int64(len(body))
	if err := adaptor.SetupRequestHeader(c, &req.Header, info); err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	for key, value := range headers {
		req.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// Provider 明确拒绝（响应已收到，无上游任务）：终态失败，不重发。
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("provider rejected the image request with HTTP %d", resp.StatusCode),
			types.ErrorCodeBadResponseStatusCode,
			resp.StatusCode,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return responseBody, nil
}

// storeImageResults uploads normalized result bytes into private OSS and
// registers each artifact durably right after its upload（评审 S6：逐图登记 +
// 确定性键 images/tasks/{taskID}/result-N，崩溃后可续传/补登记）。
func storeImageResults(ctx context.Context, task *model.Task, count int, usage *dto.Usage, fetch func(index int) ([]byte, string, error)) ([]model.TaskImageArtifact, error) {
	manifest := make([]model.TaskImageArtifact, count)
	for index := range manifest {
		key, err := service.BuildImageTaskObjectKey(task.TaskID, fmt.Sprintf("result-%d", index))
		if err != nil {
			return nil, err
		}
		manifest[index].ObjectKey = key
	}
	won, err := model.RecordImageTaskGeneration(task, manifest, usage)
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, errors.New("image execution lease lost")
	}
	artifacts := make([]model.TaskImageArtifact, 0, count)
	var deliveryErr error
	for index, planned := range manifest {
		if ctx.Err() != nil {
			return artifacts, ctx.Err()
		}
		exists, err := service.HeadImageObject(ctx, planned.ObjectKey)
		if err != nil {
			deliveryErr = errors.New("image storage is unavailable")
			continue
		}
		artifact := planned
		if !exists {
			data, mimeType, err := fetch(index)
			if err != nil {
				deliveryErr = errors.New("image result could not be read")
				continue
			}
			if mimeType == "" || !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
				mimeType = http.DetectContentType(data)
			}
			// Storage retries are bounded and reuse the same bytes/key; never rerun generation.
			for attempt := 0; attempt < 3; attempt++ {
				_, err = service.PutImageObject(ctx, planned.ObjectKey, mimeType, data)
				if err == nil || ctx.Err() != nil {
					break
				}
				if attempt < 2 {
					timer := time.NewTimer(time.Duration(attempt+1) * 500 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						return artifacts, ctx.Err()
					case <-timer.C:
					}
				}
			}
			if err != nil {
				deliveryErr = errors.New("image result could not be stored")
				continue
			}
			artifact.MimeType, artifact.Size = mimeType, int64(len(data))
		}
		artifacts = append(artifacts, artifact)
		won, err := model.AppendImageTaskArtifact(task, artifact)
		if err != nil {
			// Keep storing the other already-generated images; the persisted manifest
			// lets a later worker register them without another Provider POST.
			deliveryErr = errors.New("image artifact registration did not commit")
			continue
		}
		if !won {
			return artifacts, errors.New("image execution lease lost")
		}
	}
	return artifacts, deliveryErr
}

// downloadImageURL fetches one provider result URL with bounded size.
// 评审 S11：Provider 返回的任意媒体 URL 必须走 SSRF 防护客户端（拨号时
// 校验内网/回环目标）；该边界不得复用管理员上游连接客户端。
func downloadImageURL(ctx context.Context, imageURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	bounded := *service.GetSSRFProtectedHTTPClient()
	bounded.Timeout = 60 * time.Second
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("result download returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageResultBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxImageResultBytes {
		return nil, "", errors.New("result image exceeds the storage size limit")
	}
	mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	return data, mimeType, nil
}

// executorTransportOutcome maps pre-result transport failures：R7 要求发送
// 后一切不可判定结果按待核实处理，不自动重发或退款。
func executorTransportOutcome(apiErr *types.NewAPIError) service.ImageTaskExecution {
	if apiErr == nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "unknown_outcome"}
	}
	if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "provider_rejected"}
	}
	return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "transport_outcome_unknown"}
}

func executorFailureFromError(err error) service.ImageTaskExecution {
	var apiErr *types.NewAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "contract_rejected"}
	}
	return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeFailure, FailureCode: "request_build_failed"}
}

// ResumeImageTaskPoll 恢复一个已持久化可信上游任务 ID 的待核实任务：
// 只查询既有 Provider 任务，绝不重建（R7/§3.8 恢复表）。当前仅
// funcloud_aigc_v2 提供异步上游；无 ID 的任务不进入本路径。
func ResumeImageTaskPoll(ctx context.Context, task *model.Task) service.ImageTaskExecution {
	ctx, err := service.WithImageObjectStore(ctx)
	if err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "storage_unavailable"}
	}
	data := task.PrivateData.ImageTask
	if result, err := service.RecoverImageTaskArtifacts(ctx, task); err != nil {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "result_recovery_failed"}
	} else if result != nil {
		return *result
	}
	if data == nil || strings.TrimSpace(data.ProviderTaskID) == "" {
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "no_provider_task_id"}
	}
	info := buildFrozenImageRelayInfo(task, data)
	switch info.ChannelOtherSettings.ImageUpstreamProtocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		headers, err := restoreImageTaskHeaders(task)
		if err != nil {
			return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "headers_snapshot_unavailable"}
		}
		urls, apiErr := asyncimage.HeadlessPollOnly(ctx, info, headers, data.ProviderTaskID)
		if apiErr != nil {
			// 单次恢复查询不可采信 ≠ 业务失败：保持待核实。
			return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "resume_poll_inconclusive"}
		}
		if len(urls) == 0 {
			return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "resume_pending"}
		}
		artifacts, uploadErr := storeImageResults(ctx, task, len(urls), nil, func(index int) ([]byte, string, error) {
			return downloadImageURL(ctx, urls[index])
		})
		if uploadErr != nil {
			return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "result_store_failed"}
		}
		return service.ImageTaskExecution{
			Outcome:        service.ImageTaskOutcomeSuccess,
			ProviderTaskID: data.ProviderTaskID,
			Images:         artifacts,
		}
	default:
		return service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "resume_unsupported_protocol"}
	}
}

// newHeadlessGinContext 构造仅供 adaptor 请求头逻辑使用的最小 gin 上下文。
func newHeadlessGinContext(ctx context.Context) *gin.Context {
	headless := &gin.Context{}
	headless.Request = (&http.Request{Header: http.Header{}}).WithContext(ctx)
	headless.Request.Header.Set("Content-Type", "application/json")
	return headless
}
