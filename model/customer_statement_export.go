package model

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrCustomerStatementExportSourceUnavailable = errors.New("statement export read source is unavailable")

// OpenCustomerStatementExportSource opens a separate pool on the active primary.
func OpenCustomerStatementExportSource() (*gorm.DB, func(), error) {
	dialector := DB.Dialector
	switch primary := dialector.(type) {
	case *sqlite.Dialector:
		config := *primary
		config.Conn = nil
		separator := "?"
		if strings.Contains(config.DSN, "?") {
			separator = "&"
		}
		// The SQLite driver does not enforce sql.TxOptions.ReadOnly.
		config.DSN += separator + "_pragma=query_only(1)"
		dialector = &config
	case mysqlMigrationDialector:
		config := *primary.Config
		config.Conn = nil
		config.SkipInitializeWithVersion = true
		dialector = mysql.New(config)
	}
	db, err := gorm.Open(dialector, &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		return nil, nil, ErrCustomerStatementExportSourceUnavailable
	}
	pool, err := db.DB()
	if err != nil {
		return nil, nil, ErrCustomerStatementExportSourceUnavailable
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	return db, func() { _ = pool.Close() }, nil
}

// CheckCustomerStatementExportSource requires the publication transaction for
// every frozen generation to be visible on the read source before paging.
func CheckCustomerStatementExportSource(ctx context.Context, db *gorm.DB) error {
	ids := BillingReportFromContext(ctx).ids()
	if len(ids) == 0 {
		return nil
	}
	var count int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&billingReportGeneration{}).
			Where("id IN ? AND published = ?", ids, true).Count(&count).Error
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil || count != int64(len(ids)) {
		return ErrCustomerStatementExportSourceUnavailable
	}
	return nil
}

func CustomerStatementExportMissingDates(ctx context.Context, filters CustomerExportFilters) []string {
	if filters.BillingReportCoverage == nil {
		return nil
	}
	published := make(map[int64]bool)
	for _, generation := range BillingReportFromContext(ctx).generations {
		published[generation.Start] = true
	}
	var missing []string
	for day := range min(31, filters.BillingReportCoverage.ExpectedDays) {
		start := filters.StartTimestamp + int64(day)*billingReportDaySeconds
		if !published[start] {
			missing = append(missing, time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly))
		}
	}
	return missing
}

func readFrozenCustomerExportLines(ctx context.Context, db *gorm.DB, ids []string, userID int, filters CustomerExportFilters, cursor, ceiling int64, limit int) ([]billingReportLine, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		return nil, errors.New("statement export page size must be positive")
	}
	query := db.WithContext(ctx)
	filter := customerExportLineFilter(userID, filters)
	branches := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		part := billingReportCustomerLines(query.Model(&billingReportLine{}).Where("generation = ?", id), filter)
		if len(filters.LogTypes) > 0 {
			part = part.Where("log_type IN ?", filters.LogTypes)
		}
		if ceiling > 0 {
			part = part.Where("source_id <= ?", ceiling)
		}
		part = part.Select("source_id, customer, customer_charge, log_type").Where("source_id > ?", cursor).
			Order("source_id ASC").Limit(limit)
		branches = append(branches, "SELECT source_id, customer, customer_charge, log_type FROM (?) AS report_day")
		args = append(args, part)
	}
	var rows []billingReportLine
	sql := "SELECT source_id, customer, customer_charge, log_type FROM (" + strings.Join(branches, " UNION ALL ") + ") AS export_lines ORDER BY source_id ASC LIMIT ?"
	err := query.Raw(sql, append(args, limit)...).Find(&rows).Error
	return rows, err
}

// Read a short indexed window before applying optional filters. LIMIT on
// matching rows alone can scan a whole generation when nothing matches.
func readStatementExportCandidateIDs(ctx context.Context, db *gorm.DB, ids []string, userID int, cursor int64, limit int) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := db.WithContext(ctx)
	branches := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		part := query.Model(&billingReportLine{}).Select("source_id").
			Where("generation = ? AND user_id = ? AND source_id > ?", id, userID, cursor).
			Order("source_id ASC").Limit(limit)
		branches = append(branches, "SELECT source_id FROM (?) AS report_day")
		args = append(args, part)
	}
	var rows []int64
	sql := "SELECT source_id FROM (" + strings.Join(branches, " UNION ALL ") + ") AS export_candidates ORDER BY source_id ASC LIMIT ?"
	err := query.Raw(sql, append(args, limit)...).Scan(&rows).Error
	return rows, err
}

type CustomerStatementExportEvent struct {
	SourceID int64
	Row      CustomerExportRow
	Charge   int64 // signed settled quota
}

type CustomerStatementExportPage struct {
	Events         []CustomerStatementExportEvent
	ScannedThrough int64
	Scanned        int
	Done           bool
}

func ReadCustomerStatementExportEvents(ctx context.Context, db *gorm.DB, userID int, filters CustomerExportFilters, cursor int64, limit int) (CustomerStatementExportPage, error) {
	var page CustomerStatementExportPage
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		page, err = readCustomerStatementExportPage(ctx, tx, userID, filters, cursor, limit)
		return err
	}, &sql.TxOptions{ReadOnly: true})
	return page, err
}

func readCustomerStatementExportPage(ctx context.Context, db *gorm.DB, userID int, filters CustomerExportFilters, cursor int64, limit int) (CustomerStatementExportPage, error) {
	page := CustomerStatementExportPage{ScannedThrough: cursor}
	if limit <= 0 {
		return page, errors.New("statement export page size must be positive")
	}
	ids := BillingReportFromContext(ctx).ids()
	var ceiling int64
	bounded := len(filters.LogTypes) > 0 || filters.TokenId != nil || filters.ChannelId != nil || filters.ModelName != "" ||
		filters.BillingMode != "" || filters.TokenName != "" || filters.Group != "" || filters.Username != "" ||
		filters.RequestId != "" || filters.UpstreamRequestId != ""
	if bounded {
		candidates, err := readStatementExportCandidateIDs(ctx, db, ids, userID, cursor, limit+1)
		if err != nil {
			return page, err
		}
		page.Done = len(candidates) <= limit
		if len(candidates) == 0 {
			return page, nil
		}
		previous := cursor
		for _, sourceID := range candidates {
			if sourceID <= previous {
				return page, errors.New("duplicate source event in frozen statement export")
			}
			previous = sourceID
		}
		page.Scanned = min(limit, len(candidates))
		ceiling = candidates[page.Scanned-1]
		page.ScannedThrough = ceiling
	}
	// The lookahead detects a repeated source at a page boundary before
	// advancing the strict source_id cursor.
	rows, err := readFrozenCustomerExportLines(ctx, db, ids, userID, filters, cursor, ceiling, limit+1)
	if err != nil {
		return page, err
	}
	page.Events = make([]CustomerStatementExportEvent, 0, min(limit, len(rows)))
	for i, row := range rows {
		if row.SourceID <= cursor {
			return page, errors.New("duplicate source event in frozen statement export")
		}
		cursor = row.SourceID
		if i == limit {
			break
		}
		detail, err := row.customerDetail()
		if err != nil {
			return page, err
		}
		amount := detail.Export.Quota
		switch {
		case row.LogType == LogTypeConsume && detail.Export.EventType == "consume":
		case row.LogType == LogTypeRefund && detail.Export.EventType == "refund":
			amount = -amount
		default:
			return page, errors.New("invalid funding event in frozen statement export")
		}
		if detail.Export.Quota < 0 || amount != row.CustomerCharge {
			return page, errors.New("frozen statement export amount does not match its funding event")
		}
		page.Events = append(page.Events, CustomerStatementExportEvent{SourceID: row.SourceID, Row: detail.Export, Charge: row.CustomerCharge})
	}
	if !bounded {
		page.Done = len(rows) <= limit
		page.Scanned = len(page.Events)
		if page.Scanned > 0 {
			page.ScannedThrough = page.Events[page.Scanned-1].SourceID
		}
	}
	return page, nil
}

type CustomerStatementExportCheckpoint struct {
	Cursor      int64 `json:"cursor"`
	PendingFile bool  `json:"pending_file,omitempty"`
}

type CustomerStatementExportTotals struct {
	Charges int64 `json:"charge_quota"`
	Refunds int64 `json:"refund_quota"`
	Net     int64 `json:"net_quota"`
}

// SaveCustomerStatementExport fences both the pending reference before PUT
// and the cursor and manifest committed after upload.
func SaveCustomerStatementExport(ctx context.Context, jobID, executor string, artifact *CustomerExportArtifact, committedFile bool) error {
	raw, err := common.Marshal(artifact)
	if err != nil {
		return err
	}
	updates := map[string]any{"artifact": string(raw), "updated_at": common.GetTimestamp()}
	if committedFile {
		progress, err := common.Marshal(CustomerExportProgress{
			Scanned: artifact.LineCount, Matched: artifact.LineCount, Written: artifact.LineCount,
			Files: int64(len(artifact.Files)), CommittedAt: common.GetTimestamp(),
		})
		if err != nil {
			return err
		}
		updates["progress"] = string(progress)
	}
	result := DB.WithContext(ctx).Model(&CustomerExportJob{}).
		Where("job_id = ? AND executor = ? AND status = ? AND cancel_requested = ? AND lease_until > ?",
			jobID, executor, CustomerExportJobStatusRunning, false, common.GetTimestamp()).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCustomerExportStateConflict
	}
	return nil
}

func PauseCustomerStatementExport(ctx context.Context, jobID, executor string, now int64, expired, pressure bool) error {
	var job CustomerExportJob
	if err := DB.WithContext(ctx).Select("progress").Where("job_id = ?", jobID).Take(&job).Error; err != nil {
		return err
	}
	progress := job.DecodeProgress()
	progress.WaitingE = pressure
	raw, err := common.Marshal(progress)
	if err != nil {
		return err
	}
	query := DB.WithContext(ctx).Model(&CustomerExportJob{}).
		Where("job_id = ? AND job_type = ? AND executor = ? AND status = ? AND cancel_requested = ?",
			jobID, CustomerExportJobTypeStatementDetails, executor, CustomerExportJobStatusRunning, false)
	if expired {
		query = query.Where("lease_until < ?", now)
	} else {
		query = query.Where("lease_until > ?", now)
	}
	result := query.Updates(map[string]any{
		"status": CustomerExportJobStatusQueued, "lease_until": 0, "executor": "", "updated_at": now, "progress": string(raw),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCustomerExportStateConflict
	}
	return nil
}
