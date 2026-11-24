package model

import (
	"context"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Share archives retain the same version access rules after the source file expires.
func authorizeStatementExportVersion(ctx context.Context, job *CustomerExportJob) error {
	if job.JobType == CustomerExportJobTypeStatementDetails && job.Filters == "" {
		return nil
	}
	filters, err := job.DecodeFilters()
	if err != nil {
		return err
	}
	if job.JobType == CustomerExportJobTypeStatementArchive {
		if filters.ArchiveDraftId != nil {
			filters.StatementDraftId = *filters.ArchiveDraftId
		} else {
			// Archives created before the version was recorded must resolve their source.
			var source CustomerExportJob
			err := DB.WithContext(ctx).Select("filters").
				Where("job_id = ? AND user_id = ? AND target_user_id = ? AND job_type = ?",
					filters.ArchiveSourceJobID, job.UserId, job.TargetUserId, CustomerExportJobTypeStatementDetails).
				Take(&source).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerExportNotFound
			}
			if err != nil {
				return err
			}
			filters, err = source.DecodeFilters()
			if err != nil {
				return err
			}
		}
	}
	if filters.StatementDraftId == "" {
		return nil
	}
	version, err := CustomerStatementVersionForExport(ctx, DB, job.TargetUserId, filters)
	if err != nil {
		return err
	}
	if version.Status == BillingStatementVersionPending {
		return AuthorizeCustomerStatementsExport(ctx, job.UserId)
	}
	return nil
}

func CustomerStatementVersionForExport(ctx context.Context, db *gorm.DB, userID int, filters CustomerExportFilters) (*BillingStatementVersion, error) {
	var version BillingStatementVersion
	err := db.WithContext(ctx).
		Select("id", "status", "period_start", "period_end_exclusive", "currency", "currency_rate", "quota_per_unit").
		Where("draft_public_id = ? AND user_id = ?", filters.StatementDraftId, userID).Take(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCustomerExportNotFound
	}
	if err != nil {
		return nil, err
	}
	if (version.Status != BillingStatementVersionConfirmed && version.Status != BillingStatementVersionPending) ||
		filters.StartTimestamp < version.PeriodStart || filters.EndTimestamp > version.PeriodEndExclusive {
		return nil, ErrCustomerExportNotFound
	}
	return &version, nil
}

func readCustomerStatementVersionExportPage(ctx context.Context, db *gorm.DB, userID int, filters CustomerExportFilters, cursor int64, limit int) (CustomerStatementExportPage, error) {
	page := CustomerStatementExportPage{ScannedThrough: cursor}
	if limit <= 0 {
		return page, errors.New("statement export page size must be positive")
	}
	version, err := CustomerStatementVersionForExport(ctx, db, userID, filters)
	if err != nil {
		return page, err
	}
	base := db.WithContext(ctx).Model(&BillingStatementVersionLine{}).
		Where("version_id = ? AND created_at >= ? AND created_at < ?", version.ID, filters.StartTimestamp, filters.EndTimestamp)
	if cursor > 0 {
		var previous BillingStatementVersionLine
		if err := db.WithContext(ctx).Select("created_at").
			Where("version_id = ? AND sequence = ?", version.ID, cursor-1).Take(&previous).Error; err != nil {
			return page, err
		}
		base = base.Where("created_at >= ? AND (created_at > ? OR sequence >= ?)", previous.CreatedAt, previous.CreatedAt, cursor)
	}
	var candidates []BillingStatementVersionLine
	order := "created_at ASC, sequence ASC"
	if err := base.Session(&gorm.Session{}).Select("created_at", "sequence").
		Order(order).Limit(limit + 1).Find(&candidates).Error; err != nil {
		return page, err
	}
	page.Done = len(candidates) <= limit
	page.Scanned = min(len(candidates), limit)
	if page.Scanned == 0 {
		return page, nil
	}
	last := candidates[page.Scanned-1]
	// The one-based sequence identifies the last visited row even when timestamps
	// and insertion order differ. Resolve its timestamp when resuming.
	page.ScannedThrough = last.Sequence + 1
	query := base.Where("created_at <= ? AND (created_at < ? OR sequence <= ?)", last.CreatedAt, last.CreatedAt, last.Sequence)
	if filters.TokenId != nil {
		query = query.Where("token_id = ?", *filters.TokenId)
	}
	if len(filters.LogTypes) > 0 {
		query = query.Where("log_type IN ?", filters.LogTypes)
	}
	query = billingReportLineFilters(query, map[string]string{
		"token_name": filters.TokenName, "customer_model": filters.ModelName, "billing_mode": filters.BillingMode, "request_id": filters.RequestId,
	})
	var lines []BillingStatementVersionLine
	if err := query.Select("sequence", "facts", "log_type", "quota").Order(order).Find(&lines).Error; err != nil {
		return page, err
	}
	for _, line := range lines {
		var row CustomerExportRow
		if err := common.UnmarshalJsonStr(line.Facts, &row); err != nil {
			return page, err
		}
		amount, err := customerExportEventCharge(row, line.LogType)
		if err != nil || row.Quota != line.Quota {
			return page, errors.New("invalid funding event in statement version export")
		}
		page.Events = append(page.Events, CustomerStatementExportEvent{SourceID: line.Sequence + 1, Row: row, Charge: amount})
	}
	return page, nil
}

func customerExportEventCharge(row CustomerExportRow, logType int) (int64, error) {
	if row.Quota >= 0 {
		switch {
		case logType == LogTypeConsume && row.EventType == "consume":
			return row.Quota, nil
		case logType == LogTypeRefund && row.EventType == "refund":
			return -row.Quota, nil
		}
	}
	return 0, errors.New("invalid funding event in frozen statement export")
}

func (report *BillingReportManifest) VersionForDays(start, endExclusive int64) string {
	var ids []string
	for _, generation := range report.generations {
		if generation.Start >= start && generation.Start < endExclusive {
			ids = append(ids, generation.ID)
		}
	}
	if len(ids) == 0 {
		return "-"
	}
	return strings.Join(ids, ",")
}
