package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

// DailyReportSourceBacklog compares pending report metadata with the cutoff
// of the active log cleanup, without reading or changing the source logs.
func DailyReportSourceBacklog(ctx context.Context, selected int64) (oldest, cleanupBefore int64, err error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	if err = db.Raw(`
		SELECT COALESCE(MIN(start), 0)
		FROM (
			SELECT start FROM daily_report_stages
			UNION ALL
			SELECT start FROM billing_report_rebuilds
		) AS pending`).Scan(&oldest).Error; err != nil {
		return 0, 0, err
	}
	if oldest == 0 || (selected > 0 && selected < oldest) {
		oldest = selected
	}
	var tasks []SystemTask
	if err = db.Select("payload").Where("type = ? AND status IN ?", SystemTaskTypeLogCleanup, activeSystemTaskStatuses()).
		Order("id DESC").Limit(1).Find(&tasks).Error; err != nil || len(tasks) == 0 {
		return oldest, 0, err
	}
	var payload struct {
		TargetTimestamp int64 `json:"target_timestamp"`
	}
	err = common.UnmarshalJsonStr(tasks[0].Payload, &payload)
	return oldest, payload.TargetTimestamp, err
}
