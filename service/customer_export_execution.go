package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func customerExportTemporaryRoot() string { return filepath.Join(os.TempDir(), "customer-export-work") }

// Process crashes bypass deferred cleanup. Only this application's private
// directory is inspected, in bounded batches; active jobs retain their files.
func cleanupCustomerExportTemporaryFiles(ctx context.Context) {
	dir, err := os.Open(customerExportTemporaryRoot())
	if err != nil {
		return
	}
	defer dir.Close()
	entries, _ := dir.ReadDir(20)
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "cex_") {
			continue
		}
		job, err := model.GetCustomerExportJob(entry.Name())
		if err != nil {
			if !errors.Is(err, model.ErrCustomerExportNotFound) {
				continue
			}
			info, err := entry.Info()
			if err != nil || time.Since(info.ModTime()) < customerExportRecordRetention {
				continue
			}
		} else if job.Status == model.CustomerExportJobStatusQueued || job.Status == model.CustomerExportJobStatusRunning || job.Status == model.CustomerExportJobStatusCancelWait {
			continue
		}
		_ = os.RemoveAll(filepath.Join(customerExportTemporaryRoot(), entry.Name()))
	}
}

// One context owns the entire read/write/upload lifetime. The monitor is
// joined before the caller releases the slot, including on cancellation.
func monitorCustomerExport(parent context.Context, job *model.CustomerExportJob, executor string) (context.Context, func()) {
	budget, stopBudget := context.WithTimeout(parent, customerExportJobBudget)
	ctx, cancel := context.WithCancelCause(budget)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(customerExportProgressInterval)
		defer ticker.Stop()
		for {
			checkCtx, stop := context.WithTimeout(ctx, customerExportBatchTimeout)
			err := model.CheckCustomerExportExecution(checkCtx, job.JobID, executor, common.GetTimestamp())
			stop()
			if err != nil {
				cancel(err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ctx, func() { cancel(context.Canceled); stopBudget(); <-done }
}

func customerExportSummaryGate(pressure *customerExportPressureTracker, job *model.CustomerExportJob, progress *model.CustomerExportProgress) func(context.Context) error {
	if progress == nil {
		progress = &model.CustomerExportProgress{}
	}
	return func(ctx context.Context) error {
		// At most 500 candidates per 250ms, with every auxiliary read inside
		// the batch timeout. No connection is held during either wait.
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
			pressure.step(model.CustomerExportLogDatabasePressure())
			if err := pressure.publishWaiting(ctx, job.JobID, job.Executor, progress); err != nil {
				return err
			}
			if !pressure.degraded {
				return nil
			}
			timer.Reset(customerExportPressurePause)
		}
	}
}

// Publish transitions before waiting, retaining counters and holding no connection
// during the cooldown. Both detail and summary scans use the same visible state.
func (p *customerExportPressureTracker) publishWaiting(ctx context.Context, jobID, executor string, progress *model.CustomerExportProgress) error {
	if progress.WaitingE == p.degraded {
		return nil
	}
	next := *progress
	next.WaitingE = p.degraded
	writeCtx, cancel := context.WithTimeout(ctx, customerExportBatchTimeout)
	defer cancel()
	if err := model.UpdateCustomerExportProgress(writeCtx, jobID, executor, next); err != nil {
		return err
	}
	*progress = next
	return nil
}
