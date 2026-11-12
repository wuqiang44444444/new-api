package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// 客户月账单版本的读取投影、更正草稿占用、草稿清理
// （docs/80-dev/2026-09-17 方案第 7、10.7、12、13 节）。
// 主体逻辑集中在本文件；既有文件只保留必要接线。

// BillingStatementVersionStatement 从冻结投影重建账单视图（方案 13 读路径）。
// 汇总、分组、折扣组合与数据质量全部来自生成时冻结的 JSON，不重新扫描来源。
func BillingStatementVersionStatement(v *BillingStatementVersion) (*BillingCustomerStatement, error) {
	return ReadBillingStatementProjection(v, "api_key", nil, "", "")
}

// BillingStatementVersionLineFilter 版本明细筛选；nil ID 表示不筛选，指向 0 则精确筛选零号 Key/渠道。
type BillingStatementVersionLineFilter struct {
	TokenId     *int
	ChannelId   *int
	ModelName   string
	BillingMode string
	LogType     int // 0 表示全部
	FundGroup   string
}

// ListBillingStatementVersionLines 按 version_id + sequence 稳定分页读取版本明细（方案 7/13）。
func ListBillingStatementVersionLines(ctx context.Context, versionId int64, filter BillingStatementVersionLineFilter, page int, pageSize int) ([]BillingStatementVersionLine, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 500 {
		pageSize = 500
	}
	query := billingStatementVersionLineQuery(ctx, versionId, filter)
	if filter.FundGroup != "" {
		query = query.Where(billingStatementVersionFundGroupSQL(query)+" = ?", filter.FundGroup)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var lines []BillingStatementVersionLine
	err := query.Order("sequence asc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&lines).Error
	return lines, total, err
}

func billingStatementVersionLineQuery(ctx context.Context, versionId int64, filter BillingStatementVersionLineFilter) *gorm.DB {
	query := DB.WithContext(ctx).Model(&BillingStatementVersionLine{}).Where("version_id = ?", versionId)
	if filter.TokenId != nil {
		query = query.Where("token_id = ?", *filter.TokenId)
	}
	if filter.ChannelId != nil {
		query = query.Where("channel_id = ?", *filter.ChannelId)
	}
	if filter.ModelName != "" {
		query = query.Where("customer_model = ?", filter.ModelName)
	}
	if filter.BillingMode != "" {
		query = query.Where("billing_mode = ?", filter.BillingMode)
	}
	if filter.LogType != 0 {
		query = query.Where("log_type = ?", filter.LogType)
	}
	return query
}

// ListBillingStatementArtifacts 列出版本的产物清单。
func ListBillingStatementArtifacts(ctx context.Context, versionId int64) ([]BillingStatementArtifact, error) {
	var artifacts []BillingStatementArtifact
	err := DB.WithContext(ctx).Where("version_id = ?", versionId).Order("id asc").Find(&artifacts).Error
	return artifacts, err
}

// AcquireBillingStatementCorrectionDraft 原子占用更正草稿（方案 7）：以当前确认版为基准，
// 记录客户可见原因与内部备注；要求基准版本就是该客户月当前确认版。
func AcquireBillingStatementCorrectionDraft(ctx context.Context, userId int, periodStart int64, timezone string, correctsVersionId int64, publicReason string, internalNote string, createdBy int, generation ...CustomerExportFilters) (*BillingStatementMonth, *BillingStatementVersion, error) {
	var month *BillingStatementMonth
	var draft *BillingStatementVersion
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireBillingStatementWritableTx(ctx, tx); err != nil {
			return err
		}
		m := BillingStatementMonth{}
		findErr := lockForUpdate(tx.WithContext(ctx)).
			Where("user_id = ? AND period_start = ?", userId, periodStart).First(&m).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrBillingStatementVersionConflict
		}
		if findErr != nil {
			return findErr
		}
		if m.CurrentVersionId == nil || *m.CurrentVersionId != correctsVersionId {
			return ErrBillingStatementVersionConflict
		}
		if m.ActiveDraftId != nil {
			return ErrBillingStatementVersionConflict
		}
		var base BillingStatementVersion
		if err := tx.WithContext(ctx).First(&base, correctsVersionId).Error; err != nil {
			return err
		}
		if base.Status != BillingStatementVersionConfirmed {
			return ErrBillingStatementVersionConflict
		}
		now := nowSeconds()
		d := BillingStatementVersion{
			ParserVersion:     BillingStatementParserVersion,
			DraftPublicId:     "bsv_" + common.GetRandomString(24),
			MonthId:           m.ID,
			UserId:            userId,
			PeriodStart:       periodStart,
			Timezone:          timezone,
			CorrectsVersionId: &correctsVersionId,
			Status:            BillingStatementVersionQueued,
			PublicReason:      publicReason,
			InternalNote:      internalNote,
			CreatedBy:         createdBy,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if len(generation) > 0 {
			if err := queueBillingStatementGenerationTx(tx, &d, generation[0]); err != nil {
				return err
			}
		}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
		m.ActiveDraftId = &d.ID
		m.RowVersion++
		m.UpdatedAt = now
		if err := tx.Save(&m).Error; err != nil {
			return err
		}
		if err := tx.Create(&BillingStatementAudit{
			VersionId: &d.ID, MonthId: &m.ID, Action: "correct",
			ActorId: createdBy, Reason: publicReason, Result: "draft_acquired", CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		month = &m
		draft = &d
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return month, draft, nil
}

// MarkBillingStatementDraftCleaning 把失效/失败/已取消草稿原子标记为清理中（方案 12.3）。
// 条件更新保证与确认/放弃并发时不会误标可确认草稿；返回是否命中。
func MarkBillingStatementDraftCleaning(ctx context.Context, draftPublicId string, actorId int) (bool, error) {
	var draft BillingStatementVersion
	if err := DB.WithContext(ctx).Where("draft_public_id = ?", draftPublicId).First(&draft).Error; err != nil {
		return false, err
	}
	if draft.SourceJobId != "" {
		job, err := GetCustomerExportJob(draft.SourceJobId)
		if err != nil && !errors.Is(err, ErrCustomerExportNotFound) {
			return false, err
		}
		if job != nil && (job.Status == CustomerExportJobStatusRunning || job.Status == CustomerExportJobStatusQueued || job.Status == CustomerExportJobStatusCancelWait) {
			return false, ErrBillingStatementVersionConflict
		}
	}
	if draft.Status == BillingStatementVersionCleaning {
		return true, nil
	}
	now := nowSeconds()
	res := DB.WithContext(ctx).Model(&BillingStatementVersion{}).
		Where("draft_public_id = ? AND status IN ?", draftPublicId,
			[]BillingStatementVersionStatus{BillingStatementVersionInvalid, BillingStatementVersionFailed, BillingStatementVersionCancelled}).
		Updates(map[string]any{"status": BillingStatementVersionCleaning, "updated_at": now})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	var v BillingStatementVersion
	if err := DB.WithContext(ctx).Select("id,month_id").Where("draft_public_id = ?", draftPublicId).First(&v).Error; err != nil {
		return false, err
	}
	if err := DB.WithContext(ctx).Create(&BillingStatementAudit{
		VersionId: &v.ID, MonthId: &v.MonthId, Action: "cleanup",
		ActorId: actorId, Result: "cleaning", CreatedAt: now,
	}).Error; err != nil {
		return false, err
	}
	return true, nil
}

// DeleteBillingStatementDraftFactsTx 在事务内删除清理中草稿的明细与产物清单行，
// 返回产物对象键供事务外删除对象；对象删除失败保留引用，不先删记录（方案 12.4）。
func DeleteBillingStatementDraftFactsTx(ctx context.Context, tx *gorm.DB, draftPublicId string) ([]string, error) {
	var v BillingStatementVersion
	if err := tx.WithContext(ctx).Where("draft_public_id = ?", draftPublicId).First(&v).Error; err != nil {
		return nil, err
	}
	if v.Status != BillingStatementVersionCleaning {
		return nil, fmt.Errorf("draft status %q not cleaning", v.Status)
	}
	var keys []string
	if err := tx.WithContext(ctx).Model(&BillingStatementArtifact{}).Where("version_id = ?", v.ID).Pluck("object_key", &keys).Error; err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Where("version_id = ?", v.ID).Delete(&BillingStatementVersionLine{}).Error; err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Where("version_id = ?", v.ID).Delete(&BillingStatementArtifact{}).Error; err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Model(&BillingStatementMonth{}).Where("id = ? AND active_draft_id = ?", v.MonthId, v.ID).Update("active_draft_id", nil).Error; err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Where("id = ?", v.ID).Delete(&BillingStatementVersion{}).Error; err != nil {
		return nil, err
	}
	return keys, nil
}
