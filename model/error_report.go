package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 每小时系统运行与错误邮件报告的持久化投递模型
// （docs/80-dev/2026-09-17-每小时系统运行与错误邮件报告开发方案.md 第 6 节）。
// 报告是既有错误/统计事实的投递快照，不是新的错误事实源；主库为权威，
// 投递状态不写入 ClickHouse。状态枚举是内部实现，不作为已发布协议。

const (
	// ErrorReportScheduleScopeGlobal 是调度状态的单实例作用域。
	ErrorReportScheduleScopeGlobal = "global"

	ErrorReportStatusBuilding = "building"
	ErrorReportStatusReady    = "ready"
	ErrorReportStatusFailed   = "failed"

	ErrorReportDeliveryPending   = "pending"
	ErrorReportDeliverySending   = "sending"
	ErrorReportDeliveryAccepted  = "smtp_accepted"
	ErrorReportDeliveryRetryWait = "retry_wait"
	ErrorReportDeliveryStopped   = "stopped"

	ErrorReportStopRecipientRemoved = "recipient_removed"
)

// ErrorReportSchedule 是报告调度的单实例持久状态：启用基准、下一待生成窗口。
// 首次启用的窗口基准持久化在本行，进程重启后从已持久化进度继续。
type ErrorReportSchedule struct {
	ID      int64  `json:"id" gorm:"primaryKey"`
	Scope   string `json:"scope" gorm:"type:varchar(32);uniqueIndex"`
	Enabled bool   `json:"enabled"`
	// BaseWindowStart 是首次启用时所在自然小时的起点（绝对时间戳），0 表示从未启用。
	BaseWindowStart int64 `json:"base_window_start"`
	// NextWindowStart 是下一个待生成窗口的起点（绝对时间戳），0 表示无待生成窗口。
	NextWindowStart int64  `json:"next_window_start"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint"`
	Periods         string `json:"-" gorm:"type:text"`
	DeliveryCursor  string `json:"-" gorm:"type:varchar(255)"`
}

// ErrorReport 是一小时报告的根记录；WindowStart 唯一约束保证同一窗口只有一份报告。
type ErrorReport struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	ReportID    string `json:"report_id" gorm:"type:varchar(64);uniqueIndex"`
	WindowStart int64  `json:"window_start" gorm:"bigint;uniqueIndex"`
	WindowEnd   int64  `json:"window_end" gorm:"bigint"`
	Timezone    string `json:"timezone" gorm:"type:varchar(64)"`
	Status      string `json:"status" gorm:"type:varchar(32);index"`
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

// GetErrorReportSchedule 读取单实例调度状态；不存在时返回 (nil, nil)。
func GetErrorReportSchedule() (*ErrorReportSchedule, error) {
	var schedule ErrorReportSchedule
	err := DB.Where("scope = ?", ErrorReportScheduleScopeGlobal).First(&schedule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &schedule, nil
}

// AdvanceErrorReportWindow 用条件更新把待生成窗口从 fromStart 推进到
// fromStart+3600；仅当当前值仍为 fromStart 时生效（CAS），并发节点重复
// 推进时返回 false。窗口进度只能在报告持久化可恢复后推进。
func AdvanceErrorReportWindow(fromStart int64) (bool, error) {
	result := DB.Model(&ErrorReportSchedule{}).
		Where("scope = ? AND next_window_start = ?", ErrorReportScheduleScopeGlobal, fromStart).
		Updates(map[string]any{
			"next_window_start": fromStart + 3600,
			"updated_at":        time.Now().Unix(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// GetErrorReportByWindowStart 按窗口起点取报告；不存在返回 (nil, nil)。
func GetErrorReportByWindowStart(windowStart int64) (*ErrorReport, error) {
	var report ErrorReport
	err := DB.Where("window_start = ?", windowStart).First(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &report, nil
}

// InsertErrorReportBuilding 为窗口创建 building 状态的报告行；
// 返回报告是否存在；唯一约束冲突不授予重建权，发布事务会锁定并核对状态。
func InsertErrorReportBuilding(report *ErrorReport) (bool, error) {
	err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(report).Error
	if err != nil {
		return false, err
	}
	var count int64
	if err := DB.Model(&ErrorReport{}).Where("report_id = ?", report.ReportID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// MarkErrorReportFailed 记录窗口生成失败；不回退窗口进度。
func MarkErrorReportFailed(reportID string, reason string) error {
	return DB.Model(&ErrorReport{}).
		Where("report_id = ? AND status IN ?", reportID, []string{ErrorReportStatusBuilding, ErrorReportStatusFailed}).
		Updates(map[string]any{
			"status":         ErrorReportStatusFailed,
			"failure_reason": reason,
			"updated_at":     time.Now().Unix(),
		}).Error
}

// PublishErrorReportParts 在单个事务内完成报告的发布：清空旧分卷与投递行
// （仅 building/failed 报告允许重建）、写入冻结分卷与初始投递行、把报告
// 标记为 ready 并记录总数。全部成功才提交，避免发送半份报告。
func PublishErrorReportParts(ctx context.Context, reportID string, next func() (*ErrorReportPart, error), recipients []string, totalEvents int, summary string, dataNote string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var report ErrorReport
		if err := lockForUpdate(tx).Where("report_id = ?", reportID).First(&report).Error; err != nil {
			return err
		}
		if report.Status == ErrorReportStatusReady {
			return nil
		}
		if report.Status != ErrorReportStatusBuilding && report.Status != ErrorReportStatusFailed {
			return fmt.Errorf("report %s is not rebuildable: %s", reportID, report.Status)
		}
		// Serialize publication with an administrator changing enabled periods.
		var schedule ErrorReportSchedule
		if err := lockForUpdate(tx).Where("scope = ?", ErrorReportScheduleScopeGlobal).First(&schedule).Error; err != nil {
			return err
		}
		if !schedule.Enabled {
			return errors.New("报告已停用，保留待处理窗口")
		}
		var periods []errorReportPeriod
		if err := common.UnmarshalJsonStr(schedule.Periods, &periods); err != nil {
			return err
		}
		allowed := false
		for _, period := range periods {
			if report.WindowStart >= period.Start && (period.End == 0 || report.WindowEnd <= period.End) {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("报告窗口不属于启用时段")
		}
		if err := tx.Where("report_id = ?", reportID).Delete(&ErrorReportPart{}).Error; err != nil {
			return err
		}
		if err := tx.Where("report_id = ?", reportID).Delete(&ErrorReportDelivery{}).Error; err != nil {
			return err
		}
		totalParts := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			part, err := next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			if part == nil || part.ReportID != reportID || part.PartNo != totalParts+1 {
				return errors.New("invalid error report part sequence")
			}
			if err := tx.Create(part).Error; err != nil {
				return err
			}
			for _, recipient := range recipients {
				row := &ErrorReportDelivery{ReportID: reportID, PartID: part.ID, PartNo: part.PartNo, Recipient: recipient, Status: ErrorReportDeliveryPending}
				if err := tx.Create(row).Error; err != nil {
					return err
				}
			}
			totalParts++
		}
		if totalParts == 0 {
			return errors.New("empty error report publication")
		}
		now := time.Now().Unix()
		recipientsJSON, err := common.Marshal(recipients)
		if err != nil {
			return err
		}
		return tx.Model(&ErrorReport{}).
			Where("report_id = ? AND status IN ?", reportID, []string{ErrorReportStatusBuilding, ErrorReportStatusFailed}).
			Updates(map[string]any{
				"status":         ErrorReportStatusReady,
				"generated_at":   now,
				"data_note":      dataNote,
				"total_events":   totalEvents,
				"total_parts":    totalParts,
				"recipients":     string(recipientsJSON),
				"summary":        summary,
				"failure_reason": "",
				"updated_at":     now,
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
		var schedule ErrorReportSchedule
		if err := lockForUpdate(tx).Where("scope = ?", ErrorReportScheduleScopeGlobal).First(&schedule).Error; err != nil {
			return err
		}
		// The persisted cursor prevents a large first recipient from monopolizing
		// successive runs. Only the earliest unfinished part per recipient is eligible.
		for i := 0; i < limit; i++ {
			query := tx.Table("error_report_deliveries AS d").Select("d.*").
				Joins("JOIN error_reports r ON r.report_id = d.report_id").
				Where("d.status IN ? AND d.next_attempt_at <= ?", []string{ErrorReportDeliveryPending, ErrorReportDeliveryRetryWait}, now).
				Where("d.lease_until IS NULL OR d.lease_until < ?", now).
				Where(`NOT EXISTS (SELECT 1 FROM error_report_deliveries earlier
     JOIN error_reports er ON er.report_id = earlier.report_id
     WHERE earlier.recipient = d.recipient AND earlier.status IN ?
     AND (er.window_start < r.window_start OR (er.window_start = r.window_start AND earlier.part_no < d.part_no)))`,
					[]string{ErrorReportDeliveryPending, ErrorReportDeliveryRetryWait, ErrorReportDeliverySending})
			var delivery ErrorReportDelivery
			err := query.Session(&gorm.Session{}).Where("d.recipient > ?", schedule.DeliveryCursor).Order("d.recipient asc").Take(&delivery).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = query.Order("d.recipient asc").Take(&delivery).Error
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			if err != nil {
				return err
			}
			result := tx.Model(&ErrorReportDelivery{}).Where("id = ? AND status IN ?", delivery.ID, []string{ErrorReportDeliveryPending, ErrorReportDeliveryRetryWait}).
				Updates(map[string]any{"status": ErrorReportDeliverySending, "lease_owner": owner, "lease_until": leaseUntil, "attempts": gorm.Expr("attempts + 1"), "updated_at": now})
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

// RequeueStaleErrorReportDeliveries 把租约过期的 sending 行按「结果不明」
// 恢复为 retry_wait 并立即重试；SMTP 已接受但本地写回失败的行由此允许
// 偶尔重复，进度不会永久卡死。
func RequeueStaleErrorReportDeliveries(now int64, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	var ids []int64
	if err := DB.Model(&ErrorReportDelivery{}).
		Select("id").
		Where("status = ? AND lease_until < ?", ErrorReportDeliverySending, now).
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
		Where("status = ? AND lease_until < ?", ErrorReportDeliverySending, now).
		Updates(map[string]any{
			"status":          ErrorReportDeliveryRetryWait,
			"next_attempt_at": now,
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
	if nextAttemptAt > 0 {
		updates["next_attempt_at"] = nextAttemptAt
	}
	if acceptedAt > 0 {
		updates["accepted_at"] = acceptedAt
	}
	if status != ErrorReportDeliverySending {
		updates["lease_until"] = nil
		updates["lease_owner"] = ""
	}
	result := DB.Model(&ErrorReportDelivery{}).
		Where("id = ? AND lease_owner = ? AND status = ?", id, owner, ErrorReportDeliverySending).
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
		Where("status IN ?", []string{ErrorReportDeliveryPending, ErrorReportDeliveryRetryWait})
	if len(keepList) > 0 {
		query = query.Where("recipient NOT IN ?", keepList)
	}
	result := query.Updates(map[string]any{
		"status":      ErrorReportDeliveryStopped,
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

// GetErrorReportByReportID 取报告根记录。
func GetErrorReportByReportID(reportID string) (*ErrorReport, error) {
	var report ErrorReport
	err := DB.Where("report_id = ?", reportID).First(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &report, nil
}

// ErrorReportBacklogStats 是系统任务状态用的安全汇总：待发小时数（仍有
// 非终态投递的 ready 报告数）、待发分卷数与最早积压窗口。
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
		Where("status IN ?", []string{ErrorReportDeliveryPending, ErrorReportDeliveryRetryWait, ErrorReportDeliverySending}).
		Group("report_id")
	if err := DB.Model(&ErrorReport{}).Where("report_id IN (?)", subQuery).Count(&stats.ReportsWithPending).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&ErrorReportDelivery{}).
		Where("status IN ?", []string{ErrorReportDeliveryPending, ErrorReportDeliveryRetryWait, ErrorReportDeliverySending}).
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
