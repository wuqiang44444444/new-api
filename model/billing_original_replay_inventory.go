package model

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/logger"
)

type CustomerOriginalReplayInventory struct {
	Date           string           `json:"date"`
	Version        int              `json:"replay_version"`
	UpperLogID     int64            `json:"upper_log_id"`
	Generation     string           `json:"generation,omitempty"`
	Published      bool             `json:"published"`
	CoverageGap    bool             `json:"source_coverage_gap"`
	Known          int64            `json:"already_known"`
	Replayable     int64            `json:"replayable"`
	Unavailable    map[string]int64 `json:"unavailable,omitempty"`
	EnqueueRebuild bool             `json:"enqueue_rebuild"`
}

// OpenCustomerOriginalReplayInventory opens sources without running gateway
// startup, schema migrations, scheduled jobs, or writes.
func OpenCustomerOriginalReplayInventory(ctx context.Context) (context.Context, func(), error) {
	dsn, logDSN := os.Getenv("SQL_DSN"), os.Getenv("LOG_SQL_DSN")
	if dsn == "" {
		return ctx, nil, errors.New("SQL_DSN must identify the billing database")
	}
	if strings.HasPrefix(dsn, "local") || strings.HasPrefix(logDSN, "local") {
		path := os.Getenv("SQLITE_PATH")
		uri, err := url.Parse(path)
		if err != nil || uri.Scheme != "file" || uri.Query().Get("mode") != "ro" {
			return ctx, nil, errors.New("SQLITE_PATH must be a read-only file URI")
		}
		previousPath := common.SQLitePath
		common.SQLitePath = path
		defer func() { common.SQLitePath = previousPath }()
	}
	main, mainType, err := chooseDB("SQL_DSN", false)
	if err != nil {
		return ctx, nil, errors.New("could not open the main database")
	}
	main.Logger = logger.Default.LogMode(logger.Silent)
	mainSQL, err := main.DB()
	if err != nil {
		return ctx, nil, err
	}
	mainSQL.SetMaxOpenConns(1)
	source := usageReportSource{main: main, logs: main}
	logType := mainType
	closeSource := func() { _ = mainSQL.Close() }
	if logDSN != "" {
		if isClickHouseDSN(logDSN) {
			closeSource()
			return ctx, nil, errors.New("daily billing reports require a relational log source")
		}
		logs, kind, err := chooseDB("LOG_SQL_DSN", true)
		if err != nil {
			closeSource()
			return ctx, nil, errors.New("could not open the log database")
		}
		logs.Logger = logger.Default.LogMode(logger.Silent)
		logSQL, err := logs.DB()
		if err != nil {
			closeSource()
			return ctx, nil, err
		}
		logSQL.SetMaxOpenConns(1)
		source.logs, logType = logs, kind
		closeSource = func() {
			_ = logSQL.Close()
			_ = mainSQL.Close()
		}
	}
	priorMain, priorLogs := DB, LOG_DB
	priorMainType, priorLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = source.main, source.logs
	common.SetDatabaseTypes(mainType, logType)
	initCol()
	return context.WithValue(ctx, usageReportSourceKey{}, source), func() {
		closeSource()
		DB, LOG_DB = priorMain, priorLogs
		common.SetDatabaseTypes(priorMainType, priorLogType)
		initCol()
	}, nil
}

// InspectCustomerOriginalReplay uses the report reader and resolver. The
// result is an inventory, not permission to publish without rechecking facts.
func InspectCustomerOriginalReplay(ctx context.Context, date time.Time, userID int) (CustomerOriginalReplayInventory, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	start := date.Unix()
	out := CustomerOriginalReplayInventory{Date: date.Format(time.DateOnly), Version: CustomerOriginalReplayVersion}
	var head billingReportHead
	if db.Migrator().HasTable(&billingReportHead{}) {
		if err := db.Where("start = ?", start).Find(&head).Error; err != nil {
			return out, err
		}
	}
	out.Generation = head.Generation
	out.Published = head.Generation != ""
	var err error
	out.CoverageGap, err = dailyReportKnownLogCleanup(ctx, db, start)
	if err != nil {
		return out, err
	}
	bounded, err := WithDailyReportLogBoundary(ctx)
	if err != nil {
		return out, err
	}
	out.UpperLogID, err = dailyReportLogUpperBound(bounded)
	if err != nil {
		return out, err
	}
	var published struct {
		Rows                  int64
		OriginalReplayVersion int
	}
	if out.Published {
		if err := db.Table("billing_report_generations").
			Where("id = ?", head.Generation).Take(&published).Error; err != nil {
			return out, err
		}
	}
	var sourceRows int64
	policy := BillingStatementReadPolicy{UpperLogID: &out.UpperLogID, ReadOnlySource: true, BatchTimeout: 5 * time.Second}
	err = scanUpstreamBillingFacts(bounded, UpstreamBillingDetailFilter{Start: start, End: date.AddDate(0, 0, 1).Unix() - 1},
		policy, func(log Log, fact billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
			sourceRows++
			if log.UserId <= 0 || parsed.isChannelTest || (userID > 0 && log.UserId != userID) {
				return nil
			}
			if err := resolveCustomerOriginalReplay(bounded, fact, &parsed); err != nil {
				return err
			}
			switch {
			case parsed.replayedOriginalQuota != nil && !parsed.unavailable && !parsed.fundingAdjustmentFailed:
				out.Replayable++
			case parsed.originalReplayReason != "" || parsed.replayedOriginalQuota != nil:
				if out.Unavailable == nil {
					out.Unavailable = make(map[string]int64)
				}
				reason := parsed.originalReplayReason
				if reason == "" {
					reason = BillingOriginalReplayUnverified
				}
				out.Unavailable[reason]++
			default:
				out.Known++
			}
			return nil
		})
	out.CoverageGap = out.CoverageGap || sourceRows < published.Rows
	out.EnqueueRebuild = err == nil && !out.CoverageGap && (out.Replayable > 0 || len(out.Unavailable) > 0) && published.OriginalReplayVersion < CustomerOriginalReplayVersion
	return out, err
}
