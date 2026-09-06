package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// imageTaskExecuteHandler runs the explicit image execution worker（异步图片
// 受理任务的恢复、领取与执行）。独立于 async_task_poll：旧视频轮询/退款
// 扫描按显式执行类型排除图片任务，图片阶段由本 handler 独占推进。
type imageTaskExecuteHandler struct{}

func (imageTaskExecuteHandler) Type() string { return model.SystemTaskTypeImageTask }

func (imageTaskExecuteHandler) Enabled() bool {
	return constant.UpdateTask && service.HasImageTaskWorkOut()
}

func (imageTaskExecuteHandler) Interval() time.Duration { return 10 * time.Second }

func (imageTaskExecuteHandler) NewPayload() any { return nil }

func (imageTaskExecuteHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	service.RunImageTaskWorkerOnce(ctx)
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, nil, nil)
}
