package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const CustomerExportJobTypeStatementVersion = "statement_version"

// 小范围重复占槽接线，使草稿和导出队列在同一事务受理，不改变普通导出接口。
func queueBillingStatementGenerationTx(tx *gorm.DB, v *BillingStatementVersion, filters CustomerExportFilters) error {
	jobID, err := GenerateCustomerExportJobID()
	if err != nil {
		return err
	}
	filters.StatementDraftId = v.DraftPublicId
	raw, err := common.Marshal(filters)
	if err != nil {
		return err
	}
	slot := int64(0)
	for id := int64(1); id <= CustomerExportSlotCount; id++ {
		result := tx.Model(&CustomerExportSlot{}).Where("id = ? AND job_id = ?", id, "").Updates(map[string]interface{}{"job_id": jobID, "updated_at": nowSeconds()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			slot = id
			break
		}
	}
	if slot == 0 {
		return ErrCustomerExportQueueBusy
	}
	key := fmt.Sprintf("bsv:%d:%d", v.UserId, v.PeriodStart)
	job := CustomerExportJob{JobID: jobID, UserId: v.CreatedBy, TargetUserId: v.UserId, JobType: CustomerExportJobTypeStatementVersion, Status: CustomerExportJobStatusQueued, ActiveKey: &key, SlotId: slot, Filters: string(raw)}
	if err := tx.Create(&job).Error; err != nil {
		return err
	}
	v.SourceJobId = jobID
	v.PeriodEndExclusive = filters.EndTimestamp
	v.QuotaPerUnit = filters.QuotaPerUnit
	v.Currency = filters.Currency
	v.CurrencyRate = filters.CurrencyRate
	v.Language = filters.Language
	return nil
}

// 复用导出队列的终态事务：任务成功与 pending 发布原子提交，租约/取消守卫由调用方负责。
func finishBillingStatementGenerationTx(tx *gorm.DB, jobID string, status CustomerExportJobStatus) error {
	var job CustomerExportJob
	if err := tx.Where("job_id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	if job.JobType != CustomerExportJobTypeStatementVersion {
		return nil
	}
	var v BillingStatementVersion
	if err := lockForUpdate(tx).Where("source_job_id = ?", jobID).First(&v).Error; err != nil {
		return err
	}
	if status == CustomerExportJobStatusSucceeded {
		if v.Status != BillingStatementVersionGenerating || v.ParserVersion != BillingStatementParserVersion {
			return ErrBillingStatementVersionConflict
		}
		if err := verifyBillingStatementRetentionTx(tx.Statement.Context, tx, v.UserId, v.PeriodStart); err != nil {
			return err
		}
		v.Status = BillingStatementVersionPending
	} else if v.Status == BillingStatementVersionQueued || v.Status == BillingStatementVersionGenerating {
		v.Status = BillingStatementVersionFailed
		if status == CustomerExportJobStatusCancelled {
			v.Status = BillingStatementVersionCancelled
		}
	}
	if err := tx.Model(&v).Updates(map[string]interface{}{"status": v.Status, "updated_at": nowSeconds()}).Error; err != nil {
		return err
	}
	if v.Status == BillingStatementVersionFailed || v.Status == BillingStatementVersionCancelled {
		if err := tx.Model(&BillingStatementMonth{}).Where("id = ? AND active_draft_id = ?", v.MonthId, v.ID).Update("active_draft_id", nil).Error; err != nil {
			return err
		}
	}
	return tx.Create(&BillingStatementAudit{VersionId: &v.ID, MonthId: &v.MonthId, Action: "generation", Result: string(v.Status), CreatedAt: nowSeconds()}).Error
}

func CheckBillingStatementGeneration(ctx context.Context, v *BillingStatementVersion, job *CustomerExportJob) error {
	if err := CheckCustomerExportExecution(ctx, job.JobID, job.Executor, nowSeconds()); err != nil {
		return err
	}
	var actor User
	if err := DB.WithContext(ctx).Select("role,status").First(&actor, job.UserId).Error; err != nil {
		return err
	}
	if actor.Role < common.RoleAdminUser || actor.Status != common.UserStatusEnabled {
		return errors.New("billing statement generation requires an active administrator")
	}
	var current BillingStatementVersion
	if err := DB.WithContext(ctx).Select("status, parser_version").Where("id = ?", v.ID).Take(&current).Error; err != nil {
		return err
	}
	if current.Status != BillingStatementVersionGenerating || current.ParserVersion != BillingStatementParserVersion {
		return ErrBillingStatementVersionConflict
	}
	return nil
}

// 每批短事务同时检查执行权与草稿状态，失败不保留半批明细。
func AppendBillingStatementLines(ctx context.Context, v *BillingStatementVersion, job *CustomerExportJob, lines []BillingStatementVersionLine) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current CustomerExportJob
		if err := lockForUpdate(tx).Where("job_id = ?", job.JobID).First(&current).Error; err != nil {
			return err
		}
		if current.Executor != job.Executor || current.Status != CustomerExportJobStatusRunning || current.CancelRequested || current.LeaseUntil <= nowSeconds() {
			return ErrCustomerExportStateConflict
		}
		var draft BillingStatementVersion
		if err := lockForUpdate(tx).First(&draft, v.ID).Error; err != nil {
			return err
		}
		if draft.Status != BillingStatementVersionGenerating || draft.ParserVersion != BillingStatementParserVersion {
			return ErrBillingStatementVersionConflict
		}
		return tx.Create(&lines).Error
	})
}
