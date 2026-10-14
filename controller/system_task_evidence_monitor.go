package controller

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// This separate scheduled handler still checks a hung cleanup runner. Its
// result is visible through the existing Root system-task history API.
type evidenceCleanupMonitorHandler struct{}

func (evidenceCleanupMonitorHandler) Type() string {
	return model.SystemTaskTypeTaskRequestEvidenceMonitor
}
func (evidenceCleanupMonitorHandler) Enabled() bool           { return true }
func (evidenceCleanupMonitorHandler) Interval() time.Duration { return 15 * time.Minute }
func (evidenceCleanupMonitorHandler) NewPayload() any         { return nil }
func (evidenceCleanupMonitorHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	health, err := service.CheckEvidenceCleanupHealth(common.GetTimestamp())
	status := model.SystemTaskStatusSucceeded
	category := ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		category = "health_check_failed"
	} else if len(health.Alerts) > 0 {
		status = model.SystemTaskStatusFailed
		category = strings.Join(health.Alerts, ",")
	}
	if category != "" {
		logger.LogWarn(ctx, "evidence cleanup health alert: "+category)
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, health, category); err != nil {
		common.SysError("evidence cleanup monitor finish failed")
	}
}
