package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// The shared polling lease serializes passes. The cursor is only a scan hint
// and can start over when the process changes.
var taskPollingCursor atomic.Int64

func nextTaskPollingBatch(ctx context.Context) []*model.Task {
	limit := constant.TaskQueryLimit
	if limit <= 0 {
		limit = constant.DefaultTaskQueryLimit
	}
	afterID := taskPollingCursor.Load()
	now := time.Now()
	var batch []*model.Task
	// Read at most one refill page when deferred tasks leave room in the batch.
	for pageNumber := 0; pageNumber < 2 && len(batch) < limit; pageNumber++ {
		pageSize := limit - len(batch)
		page, err := model.ListUnfinishedSyncTaskPage(ctx, afterID, pageSize)
		if err == nil && len(page) == 0 && pageNumber == 0 && afterID != 0 {
			// A full previous page may have ended exactly at the tail.
			afterID = 0
			page, err = model.ListUnfinishedSyncTaskPage(ctx, afterID, pageSize)
		}
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("List unfinished polling tasks: %v", err))
			break
		}
		for _, task := range page {
			if !taskPollingDeferredByUpstreamSchedule(ctx, task, now) {
				batch = append(batch, task)
			}
		}
		if len(page) < pageSize {
			taskPollingCursor.Store(0)
			break
		}
		afterID = page[len(page)-1].ID
		taskPollingCursor.Store(afterID)
	}
	return batch
}
