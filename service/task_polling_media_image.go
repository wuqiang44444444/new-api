package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	mediaimageprotocol "github.com/QuantumNous/new-api/relay/mediaimage"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
)

func PollMediaImageTaskOnce(ctx context.Context, publicTaskID string) (*model.Task, error) {
	task, exists, err := model.GetByOnlyTaskId(publicTaskID)
	if err != nil {
		return nil, err
	}
	if !exists || task.Platform != constant.TaskPlatformMediaImage || task.ClientProtocol != model.TaskClientProtocolOpenAIImages {
		return nil, errors.New("media image task was not found")
	}
	if task.Status.IsTerminal() {
		return task, nil
	}
	if !model.MediaImageTaskSnapshotIsCurrent(task) {
		return finalizeMediaImageTask(ctx, task, nil, nil, "media image task contract snapshot is invalid")
	}
	media := task.PrivateData.MediaImage
	if media == nil {
		return task, errors.New("media image task snapshot is missing")
	}
	if media.RequestedImageCount == 0 || media.RequestedImageCount > dto.MaxImageN {
		return finalizeMediaImageTask(ctx, task, nil, nil, "media image task requested count is invalid")
	}
	protocol, err := mediaimageprotocol.ValidateProtocol(media.Protocol)
	if err != nil {
		return finalizeMediaImageTask(ctx, task, nil, nil, err.Error())
	}
	if strings.TrimSpace(media.QueryPathTemplate) == "" {
		return finalizeMediaImageTask(ctx, task, nil, nil, "media image task query path is required")
	}
	client, err := GetHttpClientWithProxy(media.Proxy)
	if err != nil {
		return task, fmt.Errorf("create media image task proxy client: %w", err)
	}
	observation, err := mediaimageprotocol.Query(ctx, client.Do, mediaimageprotocol.QuerySpec{
		Protocol:          protocol,
		BaseURL:           media.QueryBaseURL,
		PathTemplate:      media.QueryPathTemplate,
		TaskID:            task.GetUpstreamTaskID(),
		APIKey:            task.PrivateData.Key,
		AuthType:          media.AuthType,
		AuthName:          media.AuthName,
		AuthValueTemplate: media.AuthValueTemplate,
	})
	if err != nil {
		return task, err
	}
	if !observation.Trustworthy {
		return reconcileMediaImageTaskContract(task)
	}
	media.PollAttempts++
	if observation.RequestID != "" {
		media.LastPollRequestID = observation.RequestID
	}

	switch observation.State {
	case mediaimageprotocol.StateQueued:
		return updateActiveMediaImageTask(task, model.TaskStatusQueued, "0%")
	case mediaimageprotocol.StateInProgress:
		return updateActiveMediaImageTask(task, model.TaskStatusInProgress, "50%")
	case mediaimageprotocol.StateCompleted:
		usage, err := decodeMediaImageTaskUsage(observation.Result.Usage)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("media image task %s ignored invalid usage: %s", task.TaskID, err.Error()))
			usage = nil
		}
		urls, err := mediaimageprotocol.NormalizeResultURLs(observation.Result, dto.MaxImageN)
		if err != nil {
			return finalizeMediaImageProviderContractFailure(ctx, task)
		}
		if len(urls) > int(media.RequestedImageCount) {
			logger.LogWarn(ctx, fmt.Sprintf(
				"media image task %s returned %d images for requested n=%d; failing closed",
				task.TaskID,
				len(urls),
				media.RequestedImageCount,
			))
			return finalizeMediaImageProviderContractFailure(ctx, task)
		}
		return finalizeMediaImageTask(ctx, task, urls, usage, "")
	case mediaimageprotocol.StateFailed:
		return finalizeMediaImageTask(ctx, task, nil, nil, observation.Failure)
	default:
		return reconcileMediaImageTaskContract(task)
	}
}

func decodeMediaImageTaskUsage(raw json.RawMessage) (*dto.Usage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var openAIUsage dto.Usage
	var openAIUsageErr error
	if err := common.Unmarshal(raw, &openAIUsage); err == nil {
		if normalized, err := normalizeMediaImageUsage(&openAIUsage); err == nil {
			return normalized, nil
		} else {
			openAIUsageErr = err
		}
	} else {
		openAIUsageErr = err
	}

	var geminiUsage dto.GeminiUsageMetadata
	if err := common.Unmarshal(raw, &geminiUsage); err == nil && dto.HasGeminiUsageMetadataTokens(&geminiUsage) {
		usage := relayconvert.UsageFromGeminiMetadata(&geminiUsage, 0)
		usage.UsageSemantic = dto.BillingUsageSemanticGemini
		return normalizeMediaImageUsage(usage)
	}
	if openAIUsageErr != nil {
		return nil, openAIUsageErr
	}
	return nil, errors.New("upstream media image usage is empty")
}

func updateActiveMediaImageTask(task *model.Task, status model.TaskStatus, progress string) (*model.Task, error) {
	fromStatus := task.Status
	task.Status = status
	task.Progress = progress
	if status == model.TaskStatusInProgress && task.StartTime == 0 {
		task.StartTime = common.GetTimestamp()
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil {
		return task, err
	}
	if won {
		return task, nil
	}
	current, exists, err := model.GetByOnlyTaskId(task.TaskID)
	if err != nil || !exists {
		return task, err
	}
	return current, nil
}

func finalizeMediaImageTask(ctx context.Context, task *model.Task, urls []string, usage *dto.Usage, failure string) (*model.Task, error) {
	fromStatus := task.Status
	now := common.GetTimestamp()
	task.Progress = "100%"
	task.FinishTime = now
	if failure == "" {
		task.Status = model.TaskStatusSuccess
		task.PrivateData.MediaImage.ResultURLs = append([]string(nil), urls...)
		task.PrivateData.MediaImage.Usage = usage
	} else {
		task.Status = model.TaskStatusFailure
		task.FailReason = mediaimageprotocol.SanitizeFailure(failure)
		if task.PrivateData.AsyncBilling != nil {
			task.PrivateData.AsyncBilling.Operation = "refund"
			task.PrivateData.AsyncBilling.Reason = task.FailReason
		}
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil {
		return task, err
	}
	if !won {
		current, exists, loadErr := model.GetByOnlyTaskId(task.TaskID)
		if loadErr != nil || !exists {
			return task, loadErr
		}
		return current, nil
	}
	if task.Status == model.TaskStatusSuccess {
		settleMediaImageTask(ctx, task)
	} else if task.Quota != 0 {
		refundTaskWithReconcile(ctx, task, task.FailReason)
	}
	return task, nil
}

func UpdateMediaImageTasks(ctx context.Context, taskM map[string]*model.Task) error {
	var firstErr error
	seen := make(map[string]struct{}, len(taskM))
	for _, task := range taskM {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if task == nil {
			continue
		}
		if _, ok := seen[task.TaskID]; ok {
			continue
		}
		seen[task.TaskID] = struct{}{}
		if _, err := PollMediaImageTaskOnce(ctx, task.TaskID); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("media image task %s polling failed: %s", task.TaskID, err.Error()))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
