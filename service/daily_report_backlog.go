package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var nextDailyReportBacklogObservation atomic.Int64

func observeDailyReportBacklog(ctx context.Context, selected int64) {
	now := time.Now()
	next := nextDailyReportBacklogObservation.Load()
	if now.Unix() < next || !nextDailyReportBacklogObservation.CompareAndSwap(next, now.Add(time.Hour).Unix()) {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	oldest, cutoff, err := model.DailyReportSourceBacklog(readCtx, selected)
	if err != nil || oldest <= 0 {
		return
	}
	day := time.Unix(oldest, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly)
	if cutoff > oldest {
		common.SysError(fmt.Sprintf("daily report source retention risk: oldest pending day %s overlaps active log cleanup", day))
	} else {
		common.SysLog(fmt.Sprintf("daily report backlog: oldest pending day %s", day))
	}
}
