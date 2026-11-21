package model

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type dailyReportLogOrderKey struct{}

func withDailyReportLogOrder(ctx context.Context) context.Context {
	return context.WithValue(ctx, dailyReportLogOrderKey{}, true)
}

type reportLogCursor struct {
	id        int64
	createdAt int64
	byTime    bool
	nextPage  time.Time
}

func newReportLogCursor(ctx context.Context, query *gorm.DB, upper int64, seekDayStart bool) (reportLogCursor, error) {
	if !seekDayStart {
		return reportLogCursor{}, nil
	}
	dailyReport, _ := ctx.Value(dailyReportLogOrderKey{}).(bool)
	if dailyReport && query.Dialector.Name() == "postgres" {
		return reportLogCursor{byTime: true}, nil
	}
	id, err := dailyReportFirstLogCursor(ctx, query, upper, 5*time.Second)
	return reportLogCursor{id: id}, err
}

func (c reportLogCursor) more(upper int64) bool {
	// The highest ID can occur anywhere in timestamp order.
	return upper > 0 && (c.byTime || c.id < upper)
}

func (c reportLogCursor) batch(query *gorm.DB, upper int64) *gorm.DB {
	query = query.Limit(dailyReportReadBatchSize(query.Statement.Context, 500))
	if !c.byTime {
		return query.Where("id > ? AND id <= ?", c.id, upper).Order("id ASC")
	}
	query = query.Where("id > 0 AND id <= ?", upper)
	if c.id > 0 {
		query = query.Where("(created_at, id) > (?, ?)", c.createdAt, c.id)
	}
	return query.Order("created_at ASC, id ASC")
}

func (c *reportLogCursor) pace(ctx context.Context) error {
	if lowPriorityDailyReport(ctx) {
		if wait := time.Until(c.nextPage); wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		c.nextPage = time.Now().Add(20 * time.Millisecond)
	}
	return ctx.Err()
}
