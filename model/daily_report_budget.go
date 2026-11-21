package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrDailyReportResourcePressure = errors.New("daily report yielded to database pressure")

// Cleanup runs under the report worker's lease. Keep time above the maximum
// task duration for a former owner to stop before touching unstaged data.
const dailyReportAbandonedGraceSeconds = 3600

type dailyReportWriteBudgetKey struct{}
type dailyReportValidationGateKey struct{}
type dailyReportFinalValidationKey struct{}
type dailyReportLowPriorityKey struct{}
type dailyReportCandidateLeaseKey struct{}
type dailyReportBatchExecutorKey struct{}

// WithDailyReportBatchExecutor applies the same database execution slot to
// coverage queries and candidate writes. Admission runs before acquiring it.
func WithDailyReportBatchExecutor(ctx context.Context, execute func(context.Context, bool, func(context.Context) error) error) context.Context {
	return context.WithValue(ctx, dailyReportBatchExecutorKey{}, execute)
}

func executeDailyReportBatch(ctx context.Context, run func(context.Context) error) error {
	if execute, ok := ctx.Value(dailyReportBatchExecutorKey{}).(func(context.Context, bool, func(context.Context) error) error); ok {
		final, _ := ctx.Value(dailyReportFinalValidationKey{}).(bool)
		return execute(ctx, !final, run)
	}
	return run(ctx)
}

// WithLowPriorityDailyReport applies the scheduled worker's small-batch limits.
func WithLowPriorityDailyReport(ctx context.Context) context.Context {
	return context.WithValue(ctx, dailyReportLowPriorityKey{}, true)
}

func lowPriorityDailyReport(ctx context.Context) bool {
	lowPriority, _ := ctx.Value(dailyReportLowPriorityKey{}).(bool)
	return lowPriority
}

func dailyReportWriteBatchSize(ctx context.Context, current int) int {
	if lowPriorityDailyReport(ctx) {
		return min(current, 25)
	}
	return current
}

func dailyReportReadBatchSize(ctx context.Context, current int) int {
	if lowPriorityDailyReport(ctx) {
		return min(current, 128)
	}
	return current
}

func dailyReportBatchTimeout(ctx context.Context) time.Duration {
	if lowPriorityDailyReport(ctx) {
		return 2 * time.Second
	}
	return 5 * time.Second
}

func withDailyReportReadDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if lowPriorityDailyReport(ctx) {
		return context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
	}
	return ctx, func() {}
}

// WithDailyReportWorkerBudget installs the same worker budget across all
// projections and retries within one task. It never runs on request writes.
func WithDailyReportWorkerBudget(ctx context.Context, gate func(context.Context) error, pace func(context.Context, int) error) context.Context {
	ctx = context.WithValue(ctx, usageReportBatchGateKey{}, gate)
	return context.WithValue(ctx, dailyReportWriteBudgetKey{}, pace)
}

// WithDailyReportValidationGate supplies a gate that yields without waiting
// once publication starts checking mutable sources.
func WithDailyReportValidationGate(ctx context.Context, gate func(context.Context) error) context.Context {
	return context.WithValue(ctx, dailyReportValidationGateKey{}, gate)
}

func beforeDailyReportWrite(ctx context.Context, rows int) error {
	if pace, ok := ctx.Value(dailyReportWriteBudgetKey{}).(func(context.Context, int) error); ok {
		return pace(ctx, rows)
	}
	return usageReportBeforeBatch(ctx)
}

func createDailyReportRows(ctx context.Context, db *gorm.DB, rows int, projection any) error {
	if err := beforeDailyReportWrite(ctx, rows); err != nil {
		return err
	}
	return executeDailyReportBatch(ctx, func(ctx context.Context) error {
		if candidate, ok := ctx.Value(dailyReportCandidateLeaseKey{}).(*billingReportCheckpoint); ok {
			return dailyReportStageTransaction(ctx, db, candidate.taskID, candidate.runnerID, func(tx *gorm.DB) error {
				return tx.Create(projection).Error
			})
		}
		writeCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
		defer cancel()
		return db.WithContext(writeCtx).Create(projection).Error
	})
}
