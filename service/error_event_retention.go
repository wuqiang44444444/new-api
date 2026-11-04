package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// Reuse the durable scheduler and its per-type lock. An hourly bounded pass is
// sufficient for diagnostics; no second cleanup queue or new database table.
type errorEventRetentionHandler struct{}

func (errorEventRetentionHandler) Type() string            { return "error_event_retention" }
func (errorEventRetentionHandler) Enabled() bool           { return errorEventRetentionDays() > 0 }
func (errorEventRetentionHandler) Interval() time.Duration { return time.Hour }
func (errorEventRetentionHandler) NewPayload() any {
	return struct {
		Cutoff int64 `json:"cutoff"`
	}{common.GetTimestamp() - int64(errorEventRetentionDays())*86400}
}

func errorEventRetentionDays() int {
	days := common.GetEnvOrDefault("ERROR_EVENTS_RETENTION_DAYS", 30)
	if days < 0 || days > 3650 {
		return 30
	}
	return days // Zero explicitly disables automatic deletion.
}

func (errorEventRetentionHandler) Run(ctx context.Context, task *model.SystemTask, runner string) {
	var payload struct {
		Cutoff int64 `json:"cutoff"`
	}
	err := common.UnmarshalJsonStr(task.Payload, &payload)
	deleted := int64(0)
	if err == nil {
		work, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		// At most 20,000 SQL rows per hourly pass; a busy backlog can wait.
		for batch := 0; batch < 20; batch++ {
			var count int64
			count, err = model.DeleteOldErrorEventBatch(work, payload.Cutoff, 1000)
			deleted += count
			if err != nil || count < 1000 || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
				break
			}
		}
	}
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status, message = model.SystemTaskStatusFailed, "error event cleanup failed"
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runner, status, map[string]any{"deleted": deleted, "cutoff": payload.Cutoff}, message); finishErr != nil {
		logger.LogWarn(ctx, "error event cleanup could not finish its task")
	}
}

func init() { RegisterSystemTaskHandler(errorEventRetentionHandler{}) }
