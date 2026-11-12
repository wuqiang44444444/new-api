package service

import (
	"context"
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

func (usageReportHandler) Run(parent context.Context, task *model.SystemTask, runnerID string) {
	if !model.DailyReportSourceSupported() {
		_ = model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, "")
		return
	}
	ctx, cancel := context.WithTimeout(parent, 16*time.Minute)
	defer cancel()
	const cpuLimit, memoryLimit = 70, 80
	var sampledAt time.Time
	gate := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Since(sampledAt) < 3*time.Second {
			return nil
		}
		for {
			begin := time.Now()
			first, _, _ := readResourceUsage(ctx)
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			second, memory, memOK := readResourceUsage(ctx)
			cpu, cpuOK := second.percent(first, time.Since(begin))
			if cpuOK && memOK && cpu < cpuLimit && memory < memoryLimit {
				sampledAt = time.Now()
				return nil
			}
			timer = time.NewTimer(5 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	if err := gate(ctx); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	sourceCtx, closeSource, err := model.OpenUsageReportSource(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	defer closeSource()
	reportErr := runDailyReport(sourceCtx, task, runnerID, gate)
	now := common.GetTimestamp()
	for range 10 {
		if err := gate(ctx); err != nil {
			break
		}
		pruneCtx, stopPrune := context.WithTimeout(sourceCtx, 3*time.Second)
		didPrune, err := model.PruneUsageReportRows(pruneCtx, now)
		if err == nil {
			billingPruned, pruneErr := model.PruneBillingReportGenerations(pruneCtx, now)
			didPrune, err = didPrune || billingPruned, pruneErr
		}
		stopPrune()
		if err != nil {
			common.SysError("usage report cleanup failed: " + err.Error())
			break
		}
		if !didPrune {
			break
		}
	}
	if err := ctx.Err(); err == nil {
		pruneCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := model.PruneDailyReportExportReferences(model.DB.WithContext(pruneCtx)); err != nil {
			common.SysError("report export reference cleanup failed: " + err.Error())
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
