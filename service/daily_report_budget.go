package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type dailyReportResumableWorkKey struct{}
type dailyReportCleanupWorkKey struct{}

const (
	reportInitialWriteRate = 25
	reportMaxWriteRate     = 800
	reportRecoveryWait     = 10 * time.Second
)

type dailyReportBudget struct {
	admission        func(context.Context) reportAdmissionDecision
	speed            adaptiveReportRate
	nextWriteAt      time.Time
	nextBatchAt      time.Time
	pressure         bool
	calibrating      bool
	recovering       bool
	recoveryWaitUsed bool
	pauseReasons     []string
}

func newDailyReportBudget(ctx context.Context) (*dailyReportBudget, error) {
	sample, err := model.DailyReportDatabaseSampler(ctx)
	if err != nil {
		return nil, err
	}
	return &dailyReportBudget{
		admission: func(ctx context.Context) reportAdmissionDecision { return checkReportAdmission(ctx, sample) },
	}, nil
}

func (b *dailyReportBudget) gate(ctx context.Context) error {
	if cleanup, _ := ctx.Value(dailyReportCleanupWorkKey{}).(bool); cleanup {
		return b.validationGate(ctx)
	}
	resumable, _ := ctx.Value(dailyReportResumableWorkKey{}).(bool)
	for {
		err := b.validationGate(ctx)
		if !errors.Is(err, model.ErrDailyReportResourcePressure) || ctx.Err() != nil {
			return err
		}
		if !resumable && !b.calibrating {
			if !b.recovering || b.recoveryWaitUsed {
				return err
			}
			b.recoveryWaitUsed = true
			return b.waitForRecovery(ctx)
		}
		if err := waitDailyReportBudget(ctx, reportObservationInterval); err != nil {
			return b.pressureContextError(err)
		}
	}
}

func (b *dailyReportBudget) waitForRecovery(ctx context.Context) error {
	deadline := time.Now().Add(reportRecoveryWait)
	recoveryCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	// Once recovery starts, both sampling and calibration share this deadline.
	for waitDailyReportBudget(recoveryCtx, reportObservationInterval) == nil && time.Now().Before(deadline) {
		err := b.validationGate(recoveryCtx)
		if recoveryCtx.Err() != nil || !time.Now().Before(deadline) {
			break
		}
		if !errors.Is(err, model.ErrDailyReportResourcePressure) || (!b.recovering && !b.calibrating) {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return b.pressureContextError(err)
	}
	logDailyReportPressure("daily report recovery wait expired", b.pauseReasons)
	return model.ErrDailyReportResourcePressure
}

func (b *dailyReportBudget) validationGate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return b.pressureContextError(err)
	}
	decision := b.admission(ctx)
	wasRecovering := b.recovering
	b.calibrating = decision.calibrating
	b.recovering = decision.recovering
	b.pauseReasons = decision.reasons
	if err := ctx.Err(); err != nil {
		return b.pressureContextError(err)
	}
	if decision.calibrating {
		b.speed.last = decision.at
		return model.ErrDailyReportResourcePressure
	}
	b.speed.advance(decision, reportInitialWriteRate, reportMaxWriteRate)
	paused := decision.pressure == reportPause
	if paused && (!b.pressure || wasRecovering && !decision.recovering) {
		logDailyReportPressure("daily report yielded to resource pressure", decision.reasons)
	} else if !paused && b.pressure {
		common.SysLog("daily report resumed after resource pressure")
	}
	b.pressure = paused
	if paused {
		return model.ErrDailyReportResourcePressure
	}
	return nil
}

func logDailyReportPressure(message string, reasons []string) {
	if len(reasons) > 0 {
		message += " | " + strings.Join(reasons, " | ")
	}
	common.SysLog(message)
}

func (b *dailyReportBudget) pressureContextError(err error) error {
	if b.pressure && errors.Is(err, context.DeadlineExceeded) {
		return model.ErrDailyReportResourcePressure
	}
	return err
}

func (b *dailyReportBudget) pace(ctx context.Context, rows int) error {
	if cleanup, _ := ctx.Value(dailyReportCleanupWorkKey{}).(bool); cleanup && time.Now().Before(b.nextWriteAt) {
		if err := b.validationGate(ctx); err != nil {
			return err
		}
	}
	if err := waitDailyReportBudget(ctx, time.Until(b.nextWriteAt)); err != nil {
		return b.pressureContextError(err)
	}
	if err := b.gate(ctx); err != nil {
		return err
	}
	rate := max(reportInitialWriteRate, b.speed.rate)
	b.nextWriteAt = time.Now().Add(time.Duration(rows) * time.Second / time.Duration(rate))
	return nil
}

func waitDailyReportBudget(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
