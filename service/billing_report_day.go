package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func runDailyReport(ctx context.Context, task *model.SystemTask, runnerID string, gate func(context.Context) error) error {
	if err := gate(ctx); err != nil {
		if errors.Is(err, model.ErrDailyReportResourcePressure) {
			return nil
		}
		return err
	}
	now := time.Now().In(model.UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	probeCtx, cancelProbe := context.WithTimeout(ctx, 3*time.Second)
	if err := model.DiscoverLateBillingReportDays(probeCtx, today); err != nil {
		common.SysError("billing report late-source check failed: " + err.Error())
	}
	cancelProbe()
	replayCtx, cancelReplay := context.WithTimeout(ctx, 3*time.Second)
	_, replayErr := model.DiscoverCustomerOriginalReplayDays(replayCtx, today, now.Unix())
	cancelReplay()
	if replayErr != nil {
		common.SysError("billing original replay discovery failed: " + replayErr.Error())
	}
	selectCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	start, _, due, usageOnly, err := model.NextDailyReportDay(selectCtx, today, now.Unix())
	stop()
	if err != nil {
		return err
	}
	if !due {
		return nil
	}
	if err := gate(ctx); err != nil {
		if errors.Is(err, model.ErrDailyReportResourcePressure) {
			return nil
		}
		return err
	}
	observeDailyReportBacklog(ctx, start)
	buildCtx, err := model.WithDailyReportLogBoundary(model.StageDailyReport(ctx))
	var scopes []model.DailyReportScope
	var revisions map[int64]int64
	if err == nil {
		scopeCtx, cancel := context.WithTimeout(buildCtx, 5*time.Second)
		scopes, err = model.DailyReportScopes(scopeCtx, start, today, now.Unix(), usageOnly)
		if err == nil {
			revisions, err = model.DailyReportRebuildRevisions(scopeCtx, scopes)
		}
		cancel()
	}
	if err == nil {
		for _, scope := range scopes {
			err = prepareDailyReport(buildCtx, scope, task, runnerID, gate, revisions[scope.Start])
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = gate(ctx)
		if err == nil {
			publication := context.WithValue(buildCtx, dailyReportResumableWorkKey{}, true)
			err = model.PublishScheduledDailyReport(publication, scopes, task.TaskID, runnerID, start, revisions)
		}
	}
	if err != nil {
		if model.IsDailyReportCoverageIssue(err) {
			// Publication already recorded the exact failing stage.
			return err
		}
		waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		kinds := []string{model.DailyReportUsage}
		if !usageOnly || len(scopes) > 1 {
			kinds = append(kinds, model.DailyReportBilling)
		}
		for _, kind := range kinds {
			if deferErr := model.DeferDailyReportDate(waitCtx, start, kind, err); deferErr != nil {
				common.SysError("daily report retry could not be recorded: " + deferErr.Error())
			}
		}
		if errors.Is(err, model.ErrDailyReportDeferred) || errors.Is(err, model.ErrDailyReportResourcePressure) {
			return nil
		}
		return err
	}
	return nil
}

func prepareDailyReport(ctx context.Context, scope model.DailyReportScope, task *model.SystemTask, runnerID string, gate func(context.Context) error, requestedAt int64) error {
	if err := gate(ctx); err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	ready, err := model.GetDailyReportStage(probeCtx, scope, requestedAt)
	if err != nil || ready {
		return err
	}
	if migrated, err := model.MigrateHistoricalDailyReport(probeCtx, scope, task.TaskID, runnerID); migrated || err != nil {
		return err
	}
	cancel()
	ctx = context.WithValue(ctx, dailyReportResumableWorkKey{}, true)
	if scope.Kind == model.DailyReportUsage {
		date := time.Unix(scope.Start, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)
		return model.BuildUsageReportDay(ctx, date, task.TaskID, runnerID, gate)
	}
	return model.BuildBillingReportDay(ctx, scope.Start, task.TaskID, runnerID, gate, func(log *model.Log, row *model.CustomerExportRow) {
		attachCustomerExportBillingExplanation(log, row, common.QuotaPerUnit)
	})
}
