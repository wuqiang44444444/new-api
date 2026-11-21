package model

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
)

type DailyReportDatabaseLoad struct {
	Pool         sql.DBStats
	WALBytes     int64
	Transactions int64
	Probe        time.Duration
}

// DailyReportDatabaseSampler reads foreground pool counters without taking a
// foreground connection. The cheap probes use the worker's own connections.
func DailyReportDatabaseSampler(ctx context.Context) (func(context.Context) ([]DailyReportDatabaseLoad, error), error) {
	type source struct {
		read     *gorm.DB
		online   *sql.DB
		probeSQL string
	}
	main, err := DB.DB()
	if err != nil {
		return nil, err
	}
	sources := []source{{read: usageReportMainDB(ctx), online: main}}
	if LOG_DB != DB {
		logs, err := LOG_DB.DB()
		if err != nil {
			return nil, err
		}
		sources = append(sources, source{read: usageReportLogDB(ctx), online: logs})
	}
	return func(ctx context.Context) ([]DailyReportDatabaseLoad, error) {
		samples := make([]DailyReportDatabaseLoad, len(sources))
		for i := range sources {
			src := &sources[i]
			samples[i].Pool = src.online.Stats()
			start := time.Now()
			probeCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			var err error
			if src.probeSQL == "" {
				src.probeSQL, err = dailyReportLoadProbeSQL(src.read.WithContext(probeCtx))
			}
			if err == nil {
				err = src.read.WithContext(probeCtx).Raw(src.probeSQL).Row().Scan(&samples[i].WALBytes, &samples[i].Transactions)
			}
			cancel()
			samples[i].Probe = time.Since(start)
			if err != nil {
				return nil, err
			}
		}
		return samples, nil
	}, nil
}

func dailyReportLoadProbeSQL(db *gorm.DB) (string, error) {
	if db.Dialector.Name() != "postgres" {
		return "SELECT 0, 0", nil
	}
	var version int
	if err := db.Raw("SELECT CAST(current_setting('server_version_num') AS integer)").Scan(&version).Error; err != nil {
		return "", err
	}
	wal := "pg_wal_lsn_diff(pg_current_wal_insert_lsn(), '0/0')"
	if version < 100000 {
		wal = "pg_xlog_location_diff(pg_current_xlog_insert_location(), '0/0')"
	}
	return "SELECT CASE WHEN pg_is_in_recovery() THEN 0 ELSE CAST(" + wal + " AS bigint) END, " +
		"CASE WHEN current_setting('track_counts') = 'on' THEN xact_commit + xact_rollback ELSE -1 END " +
		"FROM pg_stat_database WHERE datname = current_database()", nil
}
