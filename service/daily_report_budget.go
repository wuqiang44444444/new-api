package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type dailyReportResumableWorkKey struct{}

const (
	reportInitialWriteRate = 25
	reportMaxWriteRate     = 800
)

type dailyReportBudget struct {
	admission   func(context.Context) reportAdmissionDecision
	speed       adaptiveReportRate
	nextWriteAt time.Time
	nextBatchAt time.Time
	pressure    bool
	calibrating bool
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
	resumable, _ := ctx.Value(dailyReportResumableWorkKey{}).(bool)
	for {
		err := b.validationGate(ctx)
		if (!resumable && !b.calibrating) || !errors.Is(err, model.ErrDailyReportResourcePressure) || ctx.Err() != nil {
			return err
		}
		if err := waitDailyReportBudget(ctx, time.Second); err != nil {
			return b.pressureContextError(err)
		}
	}
}

func (b *dailyReportBudget) validationGate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return b.pressureContextError(err)
	}
	decision := b.admission(ctx)
	b.calibrating = decision.calibrating
	if err := ctx.Err(); err != nil {
		return b.pressureContextError(err)
	}
	if decision.calibrating {
		b.speed.last = decision.at
		return model.ErrDailyReportResourcePressure
	}
	b.speed.advance(decision, reportInitialWriteRate, reportMaxWriteRate)
	paused := decision.pressure == reportPause
	if paused != b.pressure {
		if paused {
			common.SysLog("daily report yielded to resource pressure")
		} else {
			common.SysLog("daily report resumed after resource pressure")
		}
	}
	b.pressure = paused
	if paused {
		return model.ErrDailyReportResourcePressure
	}
	return nil
}

func (b *dailyReportBudget) pressureContextError(err error) error {
	if b.pressure && errors.Is(err, context.DeadlineExceeded) {
		return model.ErrDailyReportResourcePressure
	}
	return err
}

func (b *dailyReportBudget) pace(ctx context.Context, rows int) error {
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
