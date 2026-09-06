package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

// 图片异步执行 worker（§3.8/§3.10）。数据库驱动，不新增消息队列；多节点
// 通过 Task CAS 与执行槽位计数协调。Provider 执行本体由 relay 层注入，
// 避免 service → relay 循环依赖（沿用 GetTaskAdaptorFunc 模式）。

const (
	imageTaskReconcileLimit = 100
	imageTaskLeaseMarginSec = 60
)

// ImageTaskExecution is the transport-independent provider outcome.
type ImageTaskExecution struct {
	Outcome        string // success | failure | unknown
	FailureCode    string
	ProviderTaskID string
	Images         []model.TaskImageArtifact
	Usage          *dto.Usage
}

const (
	ImageTaskOutcomeSuccess = "success"
	ImageTaskOutcomeFailure = "failure"
	ImageTaskOutcomeUnknown = "unknown"
)

// ImageTaskExecuteFunc persists Provider evidence and objects; the worker owns claims, terminal state and billing.
// Injected from main (relay.ExecuteImageTask)。
var ImageTaskExecuteFunc func(ctx context.Context, task *model.Task) ImageTaskExecution

// ImageTaskResumePollFunc 恢复查询已持久化可信上游任务 ID 的待核实任务
// （只查询不重建）；由 main 注入（relay.ResumeImageTaskPoll，评审 S6）。
var ImageTaskResumePollFunc func(ctx context.Context, task *model.Task) ImageTaskExecution

// RunImageTaskWorkerOnce performs one reconcile + claim + execute pass.
func RunImageTaskWorkerOnce(ctx context.Context) {
	if ImageTaskExecuteFunc == nil {
		return
	}
	config := system_setting.LoadImageTaskConfig()
	now := time.Now().Unix()

	if err := model.RebuildImageTaskSlots(); err != nil {
		logger.LogWarn(ctx, "image task slot rebuild failed: "+err.Error())
	}
	reconcileImageTaskQueues(ctx, now)
	reconcileImageTaskLeases(ctx, now, config)
	resumeImageTaskUnknowns(ctx, config)
	executeQueuedImageTasks(ctx, config)
}

// resumeImageTaskUnknowns recovers stored objects or queries an existing Provider task.
// Every recovery owns an execution slot and a bounded lease; it never sends a create POST.
func resumeImageTaskUnknowns(ctx context.Context, config system_setting.ImageTaskConfig) {
	if ImageTaskResumePollFunc == nil {
		return
	}
	tasks := model.GetReconcilableImageTasks(config.WorkerBatch)
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		claimed, won, err := model.ClaimImageTaskRecovery(task.TaskID,
			model.ImageTaskExecutionScopeGlobal(), config.ExecConcurrency,
			model.ImageTaskExecutionScopeChannel(task.ChannelId), config.ChannelExec)
		if err != nil || !won {
			continue
		}
		task = claimed
		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			pollCtx, cancel := context.WithTimeout(ctx, time.Duration(config.StoreSeconds)*time.Second)
			result := ImageTaskResumePollFunc(pollCtx, task)
			cancel()
			switch result.Outcome {
			case ImageTaskOutcomeSuccess:
				if won, err := model.FinishImageTaskSuccess(task, result.Images, result.Usage); err == nil && won {
					settleImageTaskBilling(ctx, task)
					logger.LogInfo(ctx, fmt.Sprintf("image task %s recovered from stored result evidence", task.TaskID))
				}
			default:
				// 仍处理中/不可采信：释放执行槽，保持待核实与资金占用。
				_, _ = model.FinishImageTaskFailure(task, model.TaskStatusReconciliationRequired, result.FailureCode)
			}
		})
	}
}

func reconcileImageTaskQueues(ctx context.Context, now int64) {
	tasks := model.GetExpiredQueuedImageTasks(now, imageTaskReconcileLimit)
	for _, task := range tasks {
		won, err := model.FinishImageTaskFailure(task, model.TaskStatusFailure, "queue_expired")
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s queue expiry failed: %s", task.TaskID, err.Error()))
			continue
		}
		if won {
			settleImageTaskBilling(ctx, task)
		}
	}
}

func reconcileImageTaskLeases(ctx context.Context, now int64, config system_setting.ImageTaskConfig) {
	deadline := now - config.ExecuteSeconds - imageTaskLeaseMarginSec - config.StoreSeconds
	tasks := model.GetStalledImageTasks(deadline, imageTaskReconcileLimit)
	for _, task := range tasks {
		data := task.PrivateData.ImageTask
		if data == nil {
			continue
		}
		if data.SentAt > 0 {
			// 字节可能已发送：待核实，禁止自动重发或退款（R7/§3.8）。
			won, err := model.FinishImageTaskFailure(task, model.TaskStatusReconciliationRequired, "send_outcome_unknown")
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("image task %s unknown marking failed: %s", task.TaskID, err.Error()))
				continue
			}
			if won {
				logger.LogWarn(ctx, fmt.Sprintf("image task %s send outcome is unknown; manual reconciliation required", task.TaskID))
			}
			continue
		}
		// 已证实从未发送：释放执行槽位并回队重派。
		if _, err := model.RequeueImageTask(task); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s requeue failed: %s", task.TaskID, err.Error()))
		}
	}
}

func executeQueuedImageTasks(ctx context.Context, config system_setting.ImageTaskConfig) {
	candidates := model.GetQueuedImageTasks(config.WorkerBatch)
	if len(candidates) == 0 {
		return
	}
	// A failed storage check must not acquire a sending permit or change task facts.
	ctx, err := WithImageObjectStore(ctx)
	if err != nil || CheckImageObjectStoreReady(ctx) != nil {
		logger.LogWarn(ctx, "image execution paused: object storage is unavailable")
		return
	}
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		task, ok, err := model.ClaimImageTask(
			candidate.TaskID,
			model.ImageTaskExecutionScopeGlobal(), config.ExecConcurrency,
			model.ImageTaskExecutionScopeChannel(candidate.ChannelId), config.ChannelExec,
		)
		if err != nil && !model.IsImageTaskClaimLost(err) {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s claim failed: %s", candidate.TaskID, err.Error()))
		}
		if err != nil || !ok || task == nil {
			continue
		}
		wg.Add(1)
		gopool.Go(func() {
			defer wg.Done()
			executeImageTask(ctx, task, config)
		})
	}
	wg.Wait()
}

func executeImageTask(ctx context.Context, task *model.Task, config system_setting.ImageTaskConfig) {
	execCtx, cancel := context.WithTimeout(ctx,
		time.Duration(config.ExecuteSeconds+config.StoreSeconds)*time.Second)
	defer cancel()

	// 发送许可持久提交（SENDING 事实）之后才允许写出 Provider 请求字节。
	won, err := model.MarkImageTaskSending(task)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("image task %s send-permit commit failed: %s", task.TaskID, err.Error()))
		return
	}
	if !won {
		return
	}

	result := ImageTaskExecuteFunc(execCtx, task)
	// 评审 S6：可信上游任务 ID 取得即持久化，恢复路径据此续查不重建。
	if result.ProviderTaskID != "" && task.PrivateData.ImageTask.ProviderTaskID != result.ProviderTaskID {
		if _, err := model.MarkImageTaskProviderTaskID(task, result.ProviderTaskID); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s provider id persist failed: %s", task.TaskID, err.Error()))
		}
	}

	switch result.Outcome {
	case ImageTaskOutcomeSuccess:
		won, err := model.FinishImageTaskSuccess(task, result.Images, result.Usage)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s success commit failed: %s", task.TaskID, err.Error()))
		}
		if won {
			settleImageTaskBilling(ctx, task)
		}
	case ImageTaskOutcomeFailure:
		won, err := model.FinishImageTaskFailure(task, model.TaskStatusFailure, result.FailureCode)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s failure commit failed: %s", task.TaskID, err.Error()))
		}
		if won {
			settleImageTaskBilling(ctx, task)
		}
	default:
		won, err := model.FinishImageTaskFailure(task, model.TaskStatusReconciliationRequired, result.FailureCode)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("image task %s unknown commit failed: %s", task.TaskID, err.Error()))
		}
		if won {
			// 待核实任务保持资金与受理占用，仅释放执行槽位。
			logger.LogWarn(ctx, fmt.Sprintf("image task %s outcome unknown (%s); manual reconciliation required", task.TaskID, result.FailureCode))
		}
	}
}

// HasImageTaskWorkOut reports whether the image worker system task should run.
func HasImageTaskWorkOut() bool { return model.HasImageTaskWork() }
