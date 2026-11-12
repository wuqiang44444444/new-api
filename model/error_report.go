package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	// ErrorReportScheduleScopeGlobal 是调度状态的单实例作用域。
	ErrorReportScheduleScopeGlobal = "global"

	ErrorReportStatusBuilding = "building"
	ErrorReportStatusReady    = "ready"

	ErrorReportStopRecipientRemoved = "recipient_removed"
)

// ErrorReportSchedule tracks the next Beijing day and delivery round-robin position.
type ErrorReportSchedule struct {
	ID               int64  `gorm:"primaryKey"`
	Scope            string `gorm:"type:varchar(32);uniqueIndex"`
	DailyWindowStart int64  `gorm:"bigint"`
	UpdatedAt        int64  `gorm:"bigint"`
	DeliveryCursor   string `gorm:"type:varchar(255)"`
}

// ErrorReport is a frozen daily report or a historical hourly report.
type ErrorReport struct {
	ID                int64  `json:"id" gorm:"primaryKey"`
	ReportID          string `json:"report_id" gorm:"type:varchar(64);uniqueIndex"`
	WindowStart       int64  `json:"window_start" gorm:"bigint;uniqueIndex:idx_error_report_window,priority:1"`
	WindowEnd         int64  `json:"window_end" gorm:"bigint;uniqueIndex:idx_error_report_window,priority:2"`
	AggregatedThrough int64  `json:"-" gorm:"bigint"`
	Timezone          string `json:"timezone" gorm:"type:varchar(64)"`
	Status            string `json:"status" gorm:"type:varchar(32);index"`
	// GeneratedAt 是快照生成时刻；DataNote 是「截至生成时已入库」等数据说明。
	GeneratedAt int64  `json:"generated_at" gorm:"bigint"`
	DataNote    string `json:"data_note" gorm:"type:varchar(255)"`
	TotalEvents int    `json:"total_events"`
	TotalParts  int    `json:"total_parts"`
	// Recipients 是生成时冻结的收件人集合（JSON 数组文本）。
	Recipients string `json:"recipients" gorm:"type:text"`
	// Summary 是脱敏后的运行摘要（JSON 文本），供系统任务状态与预览使用。
	Summary       string `json:"summary" gorm:"type:text"`
	FailureReason string `json:"failure_reason" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint"`
}

// ErrorReportPart 是冻结的邮件分卷：主题与安全 HTML 正文在发布后不再变化。
type ErrorReportPart struct {
	ID         int64  `json:"id" gorm:"primaryKey"`
	ReportID   string `json:"report_id" gorm:"type:varchar(64);index:idx_error_report_part_report;uniqueIndex:idx_error_report_part_no,priority:1"`
	PartNo     int    `json:"part_no" gorm:"uniqueIndex:idx_error_report_part_no,priority:2"`
	Subject    string `json:"subject" gorm:"type:text"`
	BodyHTML   string `json:"body_html" gorm:"type:text"`
	DetailRows int    `json:"detail_rows"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`
}

// ErrorReportDelivery 是（分卷 × 收件人）粒度的投递进度；唯一约束保证
// 并发领取靠条件更新去重，而不是「先查再插」。
type ErrorReportDelivery struct {
	ID            int64  `json:"id" gorm:"primaryKey"`
	ReportID      string `json:"report_id" gorm:"type:varchar(64);index"`
	PartID        int64  `json:"part_id" gorm:"index:idx_error_report_delivery,priority:1;uniqueIndex:idx_error_report_delivery_rcpt,priority:1"`
	PartNo        int    `json:"part_no"`
	Recipient     string `json:"recipient" gorm:"type:varchar(255);uniqueIndex:idx_error_report_delivery_rcpt,priority:2;index:idx_error_report_recipient_status,priority:1"`
	Status        string `json:"status" gorm:"type:varchar(32);index;index:idx_error_report_recipient_status,priority:2"`
	Attempts      int    `json:"attempts"`
	NextAttemptAt int64  `json:"next_attempt_at" gorm:"bigint;index"`
	LeaseOwner    string `json:"lease_owner" gorm:"type:varchar(128)"`
	LeaseUntil    *int64 `json:"lease_until" gorm:"bigint"`
	LastError     string `json:"last_error" gorm:"type:varchar(512)"`
	StopReason    string `json:"stop_reason" gorm:"type:varchar(64)"`
	AcceptedAt    int64  `json:"accepted_at" gorm:"bigint"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint"`
}

func (s *ErrorReportSchedule) BeforeSave(_ *gorm.DB) error {
	if s.UpdatedAt == 0 {
		s.UpdatedAt = time.Now().Unix()
	}
	return nil
}

func (r *ErrorReport) BeforeSave(_ *gorm.DB) error {
	now := time.Now().Unix()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	return nil
}

func (p *ErrorReportPart) BeforeSave(_ *gorm.DB) error {
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}
	return nil
}

func (d *ErrorReportDelivery) BeforeSave(_ *gorm.DB) error {
	now := time.Now().Unix()
	if d.CreatedAt == 0 {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	return nil
}

// lockErrorReportSchedule serializes publishing and claims.
// SQLite reserves the writer before reading, as it has no row lock.
func lockErrorReportSchedule(tx *gorm.DB) (ErrorReportSchedule, error) {
	var schedule ErrorReportSchedule
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := tx.Model(&ErrorReportSchedule{}).Where("scope = ?", ErrorReportScheduleScopeGlobal).
			UpdateColumn("delivery_cursor", gorm.Expr("delivery_cursor")).Error; err != nil {
			return schedule, err
		}
	}
	err := lockForUpdate(tx).Where("scope = ?", ErrorReportScheduleScopeGlobal).First(&schedule).Error
	return schedule, err
}

// PublishDailyReport commits a complete frozen message and its per-address deliveries.
func PublishDailyReport(ctx context.Context, part *ErrorReportPart, recipients []string, totalEvents int, summary, dataNote string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		schedule, err := lockErrorReportSchedule(tx)
		if err != nil {
			return err
		}
		var report ErrorReport
		if err := lockForUpdate(tx).Where("report_id = ?", part.ReportID).First(&report).Error; err != nil {
			return err
		}
		if report.WindowEnd-report.WindowStart != DailyReportWindowSeconds {
			return errors.New("invalid daily report window")
		}
		if report.Status == ErrorReportStatusReady {
			return nil
		}
		if schedule.DailyWindowStart != report.WindowStart ||
			report.Status != ErrorReportStatusBuilding || report.AggregatedThrough != report.WindowEnd {
			return errors.New("daily report window is not ready for publication")
		}
		if part.PartNo != 1 {
			return errors.New("invalid daily report part number")
		}
		if err := tx.Create(part).Error; err != nil {
			return err
		}
		for _, recipient := range recipients {
			row := ErrorReportDelivery{ReportID: report.ReportID, PartID: part.ID, PartNo: part.PartNo, Recipient: recipient, Status: MailDeliveryPending}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		recipientsJSON, err := common.Marshal(recipients)
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		return tx.Model(&report).Updates(map[string]any{
			"status": ErrorReportStatusReady, "generated_at": now, "data_note": dataNote,
			"total_events": totalEvents, "total_parts": 1, "recipients": string(recipientsJSON),
			"summary": summary, "updated_at": now,
		}).Error
	})
}

// ClaimErrorReportDeliveries 领取一批到期投递：状态为 pending/retry_wait、
// 已到下次尝试时间且租约已释放。条件更新把领取与计数原子化，
// 返回的行由本执行者负责发送。
func ClaimErrorReportDeliveries(now int64, owner string, leaseUntil int64, limit int) ([]*ErrorReportDelivery, error) {
	if limit <= 0 {
		limit = 1
	}
	var deliveries []*ErrorReportDelivery
	err := DB.Transaction(func(tx *gorm.DB) error {
		schedule, err := lockErrorReportSchedule(tx)
		if err != nil {
			return err
		}
		// The persisted cursor prevents a large first recipient from monopolizing
		// successive runs. Only the earliest pending day per recipient is eligible.
		for i := 0; i < limit; i++ {
			query := tx.Table("error_report_deliveries AS d").Select("d.*").
				Joins("JOIN error_reports r ON r.report_id = d.report_id").
				Where("r.window_end - r.window_start = ?", DailyReportWindowSeconds).
				Where("d.status IN ? AND d.next_attempt_at <= ?", []string{MailDeliveryPending, MailDeliveryRetryWait}, now).
				Where("d.lease_until IS NULL OR d.lease_until < ?", now).
				Where(`NOT EXISTS (SELECT 1 FROM error_report_deliveries earlier
     JOIN error_reports er ON er.report_id = earlier.report_id
     WHERE earlier.recipient = d.recipient AND earlier.status IN ?
    AND er.window_end - er.window_start = ? AND er.window_start < r.window_start)`,
					[]string{MailDeliveryPending, MailDeliveryRetryWait, MailDeliverySending}, DailyReportWindowSeconds)
			var delivery ErrorReportDelivery
			candidate := query.Session(&gorm.Session{}).Where("d.recipient > ?", schedule.DeliveryCursor).
				Order("d.recipient asc").Limit(1).Find(&delivery)
			if candidate.Error == nil && candidate.RowsAffected == 0 && schedule.DeliveryCursor != "" {
				candidate = query.Order("d.recipient asc").Limit(1).Find(&delivery)
			}
			if candidate.Error != nil {
				return candidate.Error
			}
			if candidate.RowsAffected == 0 {
				break
			}
			result := tx.Model(&ErrorReportDelivery{}).Where("id = ? AND status IN ?", delivery.ID, []string{MailDeliveryPending, MailDeliveryRetryWait}).
				Updates(map[string]any{"status": MailDeliverySending, "lease_owner": owner, "lease_until": leaseUntil, "attempts": gorm.Expr("attempts + 1"), "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				break
			}
			if err := tx.First(&delivery, delivery.ID).Error; err != nil {
				return err
			}
			deliveries = append(deliveries, &delivery)
			schedule.DeliveryCursor = delivery.Recipient
		}
		if len(deliveries) == 0 {
			return nil
		}
		return tx.Model(&schedule).Update("delivery_cursor", schedule.DeliveryCursor).Error
	})
	if err != nil {
		return nil, err
	}
	return deliveries, nil
}

// An expired sending lease cannot establish whether the provider received the mail.
func MarkExpiredErrorReportDeliveriesUnknown(now int64, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	var ids []int64
	if err := DB.Model(&ErrorReportDelivery{}).
		Select("id").
		Where("status = ? AND lease_until < ?", MailDeliverySending, now).
		Order("id asc").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Model(&ErrorReportDelivery{}).
		Where("id IN ?", ids).
		Where("status = ? AND lease_until < ?", MailDeliverySending, now).
		Updates(map[string]any{
			"status":          MailDeliveryUnknown,
			"next_attempt_at": 0,
			"lease_owner":     "",
			"lease_until":     nil,
			"last_error":      "mail submission lease expired",
			"updated_at":      now,
		})
	return result.RowsAffected, result.Error
}

// FinishErrorReportDelivery 按领取标识写回投递结果；旧执行者无法覆盖新结果。
func FinishErrorReportDelivery(id int64, owner string, status string, lastError string, stopReason string, nextAttemptAt int64, acceptedAt int64) (bool, error) {
	updates := map[string]any{
		"status":      status,
		"last_error":  lastError,
		"stop_reason": stopReason,
		"updated_at":  time.Now().Unix(),
	}
	updates["next_attempt_at"] = nextAttemptAt
	if acceptedAt > 0 {
		updates["accepted_at"] = acceptedAt
	}
	if status != MailDeliverySending {
		updates["lease_until"] = nil
		updates["lease_owner"] = ""
	}
	result := DB.Model(&ErrorReportDelivery{}).
		Where("id = ? AND lease_owner = ? AND status = ?", id, owner, MailDeliverySending).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// StopErrorReportDeliveriesForRecipients 把非终态投递中不在保留集合内的
// 收件人标记为 stopped（配置移除而停止），返回停止数量。
func StopErrorReportDeliveriesForRecipients(keep map[string]bool) (int64, error) {
	keepList := make([]string, 0, len(keep))
	for recipient := range keep {
		keepList = append(keepList, recipient)
	}
	query := DB.Model(&ErrorReportDelivery{}).
		Where("status IN ?", []string{MailDeliveryPending, MailDeliveryRetryWait})
	if len(keepList) > 0 {
		query = query.Where("recipient NOT IN ?", keepList)
	}
	result := query.Updates(map[string]any{
		"status":      MailDeliveryStopped,
		"stop_reason": ErrorReportStopRecipientRemoved,
		"updated_at":  time.Now().Unix(),
	})
	return result.RowsAffected, result.Error
}

// GetErrorReportPartByID 取冻结分卷。
func GetErrorReportPartByID(id int64) (*ErrorReportPart, error) {
	var part ErrorReportPart
	err := DB.Where("id = ?", id).First(&part).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &part, nil
}

// ErrorReportBacklogStats summarizes pending daily deliveries for operators.
type ErrorReportBacklogStats struct {
	ReportsWithPending int64 `json:"reports_with_pending"`
	PendingDeliveries  int64 `json:"pending_deliveries"`
	EarliestWindow     int64 `json:"earliest_window"`
}

// GetErrorReportBacklogStats 汇总当前积压；无积压时 EarliestWindow 为 0。
func GetErrorReportBacklogStats() (*ErrorReportBacklogStats, error) {
	stats := &ErrorReportBacklogStats{}
	subQuery := DB.Model(&ErrorReportDelivery{}).
		Select("report_id").
		Where("status IN ?", []string{MailDeliveryPending, MailDeliveryRetryWait, MailDeliverySending}).
		Group("report_id")
	if err := DB.Model(&ErrorReport{}).Where("report_id IN (?)", subQuery).Count(&stats.ReportsWithPending).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&ErrorReportDelivery{}).
		Where("status IN ?", []string{MailDeliveryPending, MailDeliveryRetryWait, MailDeliverySending}).
		Count(&stats.PendingDeliveries).Error; err != nil {
		return nil, err
	}
	var earliest *int64
	err := DB.Model(&ErrorReport{}).
		Select("MIN(window_start)").
		Where("report_id IN (?)", subQuery).
		Scan(&earliest).Error
	if err != nil {
		return nil, err
	}
	if earliest != nil {
		stats.EarliestWindow = *earliest
	}
	return stats, nil
}
