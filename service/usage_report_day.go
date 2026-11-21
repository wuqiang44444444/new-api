package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type usageReportHandler struct{}

func (usageReportHandler) Type() string { return model.UsageReportTaskType }
func (usageReportHandler) Enabled() bool {
	return model.DailyReportSourceSupported()
}
func (usageReportHandler) Interval() time.Duration { return time.Minute }
func (usageReportHandler) NewPayload() any         { return nil }

func init() {
	RegisterSystemTaskHandler(usageReportHandler{})
}

func (handler usageReportHandler) Run(parent context.Context, task *model.SystemTask, runnerID string) {
	if !handler.Enabled() {
		_ = model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, "")
		return
	}
	// Bound the lifetime of parked in-process aggregates. Individual
	// statements and admission windows have their own, much shorter limits.
	ctx, cancel := context.WithTimeout(parent, 24*time.Hour)
	defer cancel()
	sourceCtx, closeSource, err := model.OpenUsageReportSource(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	defer closeSource()
	budget, err := newDailyReportBudget(sourceCtx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	gate := budget.gate
	sourceCtx = model.WithDailyReportWorkerBudget(sourceCtx, gate, budget.pace)
	sourceCtx = model.WithDailyReportValidationGate(sourceCtx, budget.validationGate)
	sourceCtx = model.WithDailyReportBatchExecutor(sourceCtx, budget.execute)
	sourceCtx = model.WithLowPriorityDailyReport(sourceCtx)
	reportErr := runDailyReport(sourceCtx, task, runnerID, gate)
	now := common.GetTimestamp()
	if err := gate(sourceCtx); err == nil {
		pruneCtx, stopPrune := context.WithTimeout(sourceCtx, 3*time.Second)
		_, err := model.PruneUsageReportRows(pruneCtx, now)
		if err == nil && gate(pruneCtx) == nil {
			_, err = model.PruneBillingReportGenerations(pruneCtx, now)
		}
		stopPrune()
		if err != nil && !errors.Is(err, model.ErrDailyReportResourcePressure) {
			common.SysError("daily report cleanup failed: " + err.Error())
		}
	}
	if err := gate(sourceCtx); err == nil {
		pruneCtx, cancel := context.WithTimeout(sourceCtx, 3*time.Second)
		if err := model.PruneDailyReportExportReferences(pruneCtx); err != nil {
			if !errors.Is(err, model.ErrDailyReportResourcePressure) {
				common.SysError("report export reference cleanup failed: " + err.Error())
			}
		}
		cancel()
	}
	if reportErr != nil {
		failSystemTask(task, runnerID, reportErr)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(parent, task, err)
	}
}
