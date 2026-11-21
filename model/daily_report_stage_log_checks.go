package model

import (
	"context"
	"time"
)

type dailyReportPreviousBoundary struct {
	upper int64
	rows  int64
}

type dailyReportStageLogCheck struct {
	current      dailyReportCoverage
	previous     dailyReportPreviousBoundary
	previousRows int64
}

func readDailyReportStageLogChecks(ctx context.Context, stages []dailyReportStage) ([]dailyReportStageLogCheck, error) {
	checks := make([]dailyReportStageLogCheck, len(stages))
	windows := make([]dailyReportLogWindow, len(stages))
	bounds := make(map[dailyReportLogWindow]dailyReportLogBounds)
	for i, stage := range stages {
		if !dailyReportStageVersionCurrent(stage) {
			return nil, errDailyReportSourceChanged
		}
		previous, err := previousDailyReportBoundary(ctx, stage)
		if err != nil {
			return nil, err
		}
		checks[i].previous = previous
		upper := stage.UpperLogID
		if current, ok := ctx.Value(dailyReportLogBoundaryKey{}).(int64); ok {
			upper = current
		}
		window := dailyReportLogWindow{stage.Start, upper}
		windows[i] = window
		boundary := bounds[window]
		if stage.Kind == DailyReportUsage {
			boundary.includeUsage = true
			boundary.usagePrevious = previous.upper
		} else {
			boundary.billingPrevious = previous.upper
		}
		bounds[window] = boundary
	}
	byWindow := make(map[dailyReportLogWindow]dailyReportDateLogCoverage)
	postgres := usageReportLogDB(ctx).Dialector.Name() == "postgres"
	for i, stage := range stages {
		window := windows[i]
		check := &checks[i]
		if postgres {
			results, ok := byWindow[window]
			if !ok {
				if err := usageReportBeforeBatch(ctx); err != nil {
					return nil, err
				}
				var err error
				results, err = readPostgresDailyReportLogCoverage(ctx, window, bounds[window])
				if err != nil {
					return nil, err
				}
				byWindow[window] = results
			}
			check.current, check.previousRows = results.forKind(stage.Kind)
			continue
		}
		current, err := readDailyReportDateCoverage(ctx, window.start, stage.Kind, window.upper)
		if err != nil {
			return nil, err
		}
		check.current = current
		if check.previous.upper != 0 && current.Rows == stage.SourceRows && current.Revision == stage.SourceRevision {
			if err := usageReportBeforeBatch(ctx); err != nil {
				return nil, err
			}
			countCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := dailyReportLogQuery(ctx, window.start, window.start+billingReportDaySeconds, check.previous.upper, dailyReportLogTypes(stage.Kind)).
				WithContext(countCtx).Count(&check.previousRows).Error
			cancel()
			if err != nil {
				return nil, err
			}
		}
	}
	return checks, nil
}
