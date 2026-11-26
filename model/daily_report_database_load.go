package model

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
)

type DailyReportDatabaseLoad struct {
	Pool         sql.DBStats
	WALBytes     int64
	Transactions int64
	Probe        time.Duration
}

const (
	DailyReportMainDatabaseSource = "main_db"
	DailyReportLogDatabaseSource  = "log_db"
)

// Cleanup rechecks after locking a generation. Reuse its transaction for main-
// database observations because the report worker has a single connection.
type dailyReportProbeTransactionKey struct{}

func beforeDailyReportCleanupWrite(ctx context.Context, tx *gorm.DB) error {
	return usageReportBeforeBatch(context.WithValue(ctx, dailyReportProbeTransactionKey{}, tx))
}

func dailyReportMainProbeDB(ctx context.Context, source *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(dailyReportProbeTransactionKey{}).(*gorm.DB); ok {
		return tx
	}
	return source
}

type DailyReportProbeError struct {
	Source  string
	Elapsed time.Duration
	Timeout time.Duration
	Cause   error
}

func (e *DailyReportProbeError) Error() string { return "daily report database probe failed" }
func (e *DailyReportProbeError) Unwrap() error { return e.Cause }

// DailyReportDatabaseSampler reads foreground pool counters without taking a
// foreground connection. The cheap probes use the worker's own connections.
func DailyReportDatabaseSampler(ctx context.Context) (func(context.Context) ([]DailyReportDatabaseLoad, error), error) {
	type source struct {
		name     string
		read     *gorm.DB
		online   *sql.DB
		probeSQL string
	}
	main, err := DB.DB()
	if err != nil {
		return nil, err
	}
	sources := []source{{name: DailyReportMainDatabaseSource, read: usageReportMainDB(ctx), online: main}}
	if LOG_DB != DB {
		logs, err := LOG_DB.DB()
		if err != nil {
			return nil, err
		}
		sources = append(sources, source{name: DailyReportLogDatabaseSource, read: usageReportLogDB(ctx), online: logs})
	}
	return func(ctx context.Context) ([]DailyReportDatabaseLoad, error) {
		samples := make([]DailyReportDatabaseLoad, len(sources))
		for i := range sources {
			src := &sources[i]
			read := src.read
			if i == 0 {
				read = dailyReportMainProbeDB(ctx, read)
			}
			samples[i].Pool = src.online.Stats()
			start := time.Now()
			const probeTimeout = 200 * time.Millisecond
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			var err error
			if src.probeSQL == "" {
				src.probeSQL, err = dailyReportLoadProbeSQL(read.WithContext(probeCtx))
			}
			if err == nil {
				err = read.WithContext(probeCtx).Raw(src.probeSQL).Row().Scan(&samples[i].WALBytes, &samples[i].Transactions)
			}
			// A database driver may return its own error without wrapping the deadline.
			if err != nil && probeCtx.Err() == context.DeadlineExceeded {
				err = errors.Join(err, context.DeadlineExceeded)
			}
			cancel()
			samples[i].Probe = time.Since(start)
			if err != nil {
				return nil, &DailyReportProbeError{Source: src.name, Elapsed: samples[i].Probe, Timeout: probeTimeout, Cause: err}
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
