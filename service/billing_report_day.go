package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func runDailyReport(ctx context.Context, task *model.SystemTask, runnerID string, gate func(context.Context) error, localFast bool) error {
	for followingReplay := false; ; followingReplay = true {
		more, err := runDailyReportCycle(ctx, task, runnerID, gate, followingReplay)
		if err != nil || !localFast || !more {
			return err
		}
	}
}

func runDailyReportCycle(ctx context.Context, task *model.SystemTask, runnerID string, gate func(context.Context) error, followingReplay bool) (bool, error) {
	now := time.Now().In(model.UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	probeCtx, cancelProbe := context.WithTimeout(ctx, 3*time.Second)
	if err := model.DiscoverLateBillingReportDays(probeCtx, today); err != nil {
		common.SysError("billing report late-source check failed: " + err.Error())
	}
	cancelProbe()
	replayCtx, cancelReplay := context.WithTimeout(ctx, 3*time.Second)
	replayPending, replayErr := model.DiscoverCustomerOriginalReplayDays(replayCtx, today, now.Unix())
	cancelReplay()
	if replayErr != nil {
		common.SysError("billing original replay discovery failed: " + replayErr.Error())
		replayPending = false
	}
	selectCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	start, requestedAt, due, usageOnly, err := model.NextDailyReportDay(selectCtx, today, now.Unix())
	stop()
	if err != nil {
		return false, err
	}
	if followingReplay && !replayPending && !usageOnly {
		return false, replayErr
	}
	if !due {
		return false, nil
	}
	if err := gate(ctx); err != nil {
		return false, err
	}
	buildCtx, err := model.WithDailyReportLogBoundary(model.StageDailyReport(ctx))
	if err != nil {
		return false, err
	}
	scopeCtx, cancel := context.WithTimeout(buildCtx, 5*time.Second)
	scopes, err := model.DailyReportScopes(scopeCtx, start, today, now.Unix(), usageOnly)
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
		kinds := []string{model.DailyReportUsage}
		if !usageOnly || len(scopes) > 1 {
			kinds = append(kinds, model.DailyReportBilling)
		}
		for _, kind := range kinds {
			if deferErr := model.DeferDailyReportDate(waitCtx, start, kind, err); deferErr != nil {
				common.SysError("daily report retry could not be recorded: " + deferErr.Error())
			}
		}
		if errors.Is(err, model.ErrDailyReportDeferred) {
			return false, nil
		}
		return false, err
	}
	if requestedAt != 0 {
		return replayPending, model.CompleteBillingReportRebuild(ctx, start, requestedAt)
	}
	return replayPending || usageOnly, nil
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
