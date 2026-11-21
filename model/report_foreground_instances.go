package model

import "context"

func ReportForegroundInstances(ctx context.Context, now int64) ([]SystemInstance, error) {
	var instances []SystemInstance
	err := usageReportMainDB(ctx).WithContext(ctx).
		Select("node_name", "info", "started_at", "last_seen_at").
		Where("last_seen_at >= ?", now-SystemInstanceStaleAfterSeconds).Find(&instances).Error
	return instances, err
}
