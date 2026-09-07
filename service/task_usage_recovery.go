package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// One worker issues at most ten sequential GETs per scheduler pass. The
// scheduler lease and per-task durable claim bound multi-instance overlap.
func ReconcileTaskUsage(ctx context.Context) {
	if GetTaskAdaptorFunc == nil {
		return
	}
	ids, err := model.DueTaskUsageCheckIDs(model.GetDBTimestamp(), 10)
	if err != nil {
		logger.LogWarn(ctx, "usage recovery scan failed")
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		task, review, err := model.ClaimTaskUsageCheck(id, model.GetDBTimestamp())
		if err != nil {
			logger.LogWarn(ctx, "usage recovery claim failed")
			continue
		}
		if task == nil {
			continue
		}
		if review {
			logger.LogWarn(ctx, fmt.Sprintf("task %s requires usage review; held_quota=%d", task.TaskID, task.Quota))
			continue
		}
		// Refresh mutates task; keep the claim independently for conditional completion.
		claimed := *task
		pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = RefreshVideoTask(pollCtx, task)
		cancel()
		if err := model.FinishTaskUsageCheck(&claimed, err != nil); err != nil {
			logger.LogWarn(ctx, "usage recovery completion failed")
		}
	}
}

// Seedance implements cancellation at the HTTP request boundary. Other existing
// adapters keep their published interface until they support contextual fetches.
func fetchVideoTaskWithContext(ctx context.Context, adaptor TaskPollingAdaptor, baseURL, key string, task *model.Task, proxy string) (*http.Response, error) {
	if contextual, ok := adaptor.(interface {
		FetchTaskWithContext(context.Context, string, string, *model.Task, string) (*http.Response, error)
	}); ok {
		return contextual.FetchTaskWithContext(ctx, baseURL, key, task, proxy)
	}
	return adaptor.FetchTask(baseURL, key, task, proxy)
}
