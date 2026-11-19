package model

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Materialize the date-scoped IDs before computing the minimum on PostgreSQL.
// The date/type predicate can use the date index instead of probing the primary
// key across the entire retention window. The existing bounded scan still
// performs all coverage and revision checks.
func dailyReportFirstLogCursor(ctx context.Context, query *gorm.DB, upper int64, timeout time.Duration) (int64, error) {
	if query.Dialector.Name() != "postgres" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	scoped := query.Session(&gorm.Session{}).WithContext(ctx).Select("id").Where("id > 0 AND id <= ?", upper)
	var first int64
	// OFFSET 0 prevents the optimizer from flattening the subquery.
	if err := query.Session(&gorm.Session{NewDB: true}).WithContext(ctx).
		Raw("SELECT COALESCE(MIN(id), 0) FROM (? OFFSET 0) AS day_ids", scoped).
		Scan(&first).Error; err != nil {
		return 0, err
	}
	if first == 0 {
		return upper, nil
	}
	return first - 1, nil
}
