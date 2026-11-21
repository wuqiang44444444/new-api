package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var errReportBatchDeferred = errors.New("report execution slot yielded before SQL")

const (
	reportExecutionLeaseTTL = 15 * time.Second
	reportBatchMaxTime      = 5 * time.Second
)

var reportExecutionSlot = struct {
	sync.Mutex
	owner   string
	db      *gorm.DB
	expires time.Time
}{owner: common.GetUUID()}

// Serialize heavy database batches locally and keep the same short-lived
// owner across processes sharing the system task database.
func executeSharedReportBatch(ctx context.Context, gate func(context.Context) error, run func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !reportExecutionSlot.TryLock() {
		return errReportBatchDeferred
	}
	defer reportExecutionSlot.Unlock()
	if err := gate(ctx); err != nil {
		if errors.Is(err, model.ErrDailyReportResourcePressure) {
			return errReportBatchDeferred
		}
		return err
	}
	now := time.Now()
	if reportExecutionSlot.db != model.DB || !now.Add(reportBatchMaxTime+time.Second).Before(reportExecutionSlot.expires) {
		leaseCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		ok, err := model.RenewReportExecutionLease(leaseCtx, reportExecutionSlot.owner, now, reportExecutionLeaseTTL)
		cancel()
		if err != nil || !ok {
			return errReportBatchDeferred
		}
		reportExecutionSlot.db = model.DB
		reportExecutionSlot.expires = time.Unix(now.Add(reportExecutionLeaseTTL).Unix(), 0)
	}
	if err := gate(ctx); err != nil {
		if errors.Is(err, model.ErrDailyReportResourcePressure) {
			return errReportBatchDeferred
		}
		return err
	}
	batchCtx, cancel := context.WithTimeout(ctx, reportBatchMaxTime)
	defer cancel()
	return run(batchCtx)
}

func (b *dailyReportBudget) execute(ctx context.Context, allowPark bool, run func(context.Context) error) error {
	resumable, _ := ctx.Value(dailyReportResumableWorkKey{}).(bool)
	if allowPark && resumable {
		if err := waitDailyReportBudget(ctx, time.Until(b.nextBatchAt)); err != nil {
			return b.pressureContextError(err)
		}
	}
	for {
		err := executeSharedReportBatch(ctx, b.validationGate, func(ctx context.Context) error {
			started := time.Now()
			err := run(ctx)
			if allowPark && resumable {
				// Yield in proportion to database occupancy. Reads and wide
				// writes both slow down when the foreground budget shrinks.
				elapsed := time.Since(started)
				rate := max(reportInitialWriteRate, b.speed.rate)
				b.nextBatchAt = time.Now().Add(min(250*time.Millisecond, elapsed*time.Duration(2*reportInitialWriteRate)/time.Duration(rate)))
			}
			return err
		})
		if !errors.Is(err, errReportBatchDeferred) {
			return err
		}
		if !allowPark || !resumable {
			return model.ErrDailyReportResourcePressure
		}
		if err := waitDailyReportBudget(ctx, 100*time.Millisecond); err != nil {
			return b.pressureContextError(err)
		}
	}
}
