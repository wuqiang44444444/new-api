package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func runDailyReport(ctx context.Context, task *model.SystemTask, runnerID string, gate func(context.Context) error) error {
	now := time.Now().In(model.UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	probeCtx, cancelProbe := context.WithTimeout(ctx, 3*time.Second)
	if err := model.DiscoverLateBillingReportDays(probeCtx, today); err != nil {
		common.SysError("billing report late-source check failed: " + err.Error())
	}
	cancelProbe()
	selectCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	start, requestedAt, due, err := model.NextDailyReportDay(selectCtx, today, now.Unix())
	stop()
	if err != nil || !due {
		return err
	}
	if err := gate(ctx); err != nil {
		return err
	}
	buildCtx, err := model.WithDailyReportLogBoundary(model.StageDailyReport(ctx))
	if err != nil {
		return err
	}
	scopeCtx, cancel := context.WithTimeout(buildCtx, 5*time.Second)
	scopes, err := model.DailyReportScopes(scopeCtx, start, today, now.Unix())
	cancel()
	if err == nil {
		for _, scope := range scopes {
			var revision int64
			if scope.Start == start {
				revision = requestedAt
			}
			err = prepareDailyReport(buildCtx, scope, task, runnerID, gate, revision)
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = model.PublishScheduledDailyReport(buildCtx, scopes, task.TaskID, runnerID, start)
	}
	if err != nil {
		waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		// Both selectors must be able to advance past an unavailable day.
		for _, kind := range []string{model.DailyReportBilling, model.DailyReportUsage} {
			if deferErr := model.DeferDailyReportDate(waitCtx, start, kind, err); deferErr != nil {
				common.SysError("daily report retry could not be recorded: " + deferErr.Error())
			}
		}
		if errors.Is(err, model.ErrDailyReportDeferred) {
			return nil
		}
		return err
	}
	if requestedAt != 0 {
		return model.CompleteBillingReportRebuild(ctx, start, requestedAt)
	}
	return nil
}

func prepareDailyReport(ctx context.Context, scope model.DailyReportScope, task *model.SystemTask, runnerID string, gate func(context.Context) error, requestedAt int64) error {
	if err := gate(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	ready, err := model.GetDailyReportStage(ctx, scope, requestedAt)
	if err != nil || ready {
		return err
	}
	if scope.Kind == model.DailyReportUsage {
		date := time.Unix(scope.Start, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)
		return model.BuildUsageReportDay(ctx, date, task.TaskID, runnerID, gate)
	}
	return model.BuildBillingReportDay(ctx, scope.Start, task.TaskID, runnerID, gate, func(log *model.Log, row *model.CustomerExportRow) {
		attachCustomerExportBillingExplanation(log, row, common.QuotaPerUnit)
	})
}
