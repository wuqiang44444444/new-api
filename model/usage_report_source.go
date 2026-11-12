package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const dailyReportRefreshSeconds = 180 * 60

func DailyReportSourceSupported() bool {
	return !common.UsingLogDatabase(common.DatabaseTypeClickHouse)
}

type usageReportSourceKey struct{}

type usageReportSource struct {
	main *gorm.DB
	logs *gorm.DB
}

// OpenUsageReportSource uses a dedicated, single-connection read pool for
// each source, even when the gateway and report share a database.
func OpenUsageReportSource(ctx context.Context) (context.Context, func(), error) {
	main, _, err := chooseDB("SQL_DSN", false)
	if err != nil {
		return ctx, nil, err
	}
	mainSQL, err := main.DB()
	if err != nil {
		return ctx, nil, err
	}
	mainSQL.SetMaxOpenConns(1)
	mainSQL.SetMaxIdleConns(1)
	source := usageReportSource{main: main, logs: main}
	closeSource := func() { _ = mainSQL.Close() }
	if LOG_DB != DB {
		logs, _, err := chooseDB("LOG_SQL_DSN", true)
		if err != nil {
			closeSource()
			return ctx, nil, err
		}
		logSQL, err := logs.DB()
		if err != nil {
			closeSource()
			return ctx, nil, err
		}
		logSQL.SetMaxOpenConns(1)
		logSQL.SetMaxIdleConns(1)
		source.logs = logs
		closeSource = func() {
			_ = logSQL.Close()
			_ = mainSQL.Close()
		}
	}
	return context.WithValue(ctx, usageReportSourceKey{}, source), closeSource, nil
}

func usageReportMainDB(ctx context.Context) *gorm.DB {
	if source, ok := ctx.Value(usageReportSourceKey{}).(usageReportSource); ok {
		return source.main
	}
	return DB
}

func usageReportLogDB(ctx context.Context) *gorm.DB {
	if source, ok := ctx.Value(usageReportSourceKey{}).(usageReportSource); ok {
		return source.logs
	}
	return LOG_DB
}
