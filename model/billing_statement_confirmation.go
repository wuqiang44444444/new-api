package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrBillingStatementSourceIncomplete = errors.New("billing statement source retention is not verified complete")

type BillingStatementIntegrity struct {
	Rows   int64 `json:"rows"`
	Gross  int64 `json:"gross"`
	Refund int64 `json:"refund"`
}

// 核验只接受已建立的来源保留事实，不把“现在的明细与汇总相等”当作历史完整证明。
func VerifyBillingStatementRetention(ctx context.Context, userId int, periodStart int64) error {
	return verifyBillingStatementRetentionTx(ctx, DB, userId, periodStart)
}
func verifyBillingStatementRetentionTx(ctx context.Context, tx *gorm.DB, userId int, periodStart int64) error {
	var retention BillingStatementRetention
	err := lockForUpdate(tx.WithContext(ctx)).Where("user_id = ? AND period_start = ?", userId, periodStart).First(&retention).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrBillingStatementSourceIncomplete
	}
	if err != nil {
		return err
	}
	if retention.Status != BillingStatementRetentionIntact && retention.Status != BillingStatementRetentionNone {
		return ErrBillingStatementSourceIncomplete
	}
	return verifyBillingSourceAttestation(ctx, tx, retention)
}

func verifyBillingStatementConfirmationTx(ctx context.Context, tx *gorm.DB, v *BillingStatementVersion, acknowledged string) error {
	if err := verifyBillingStatementRetentionTx(ctx, tx, v.UserId, v.PeriodStart); err != nil {
		return err
	}
	var integrity BillingStatementIntegrity
	if err := common.UnmarshalJsonStr(v.Integrity, &integrity); err != nil {
		return errors.New("statement integrity is not verified")
	}
	statement, err := BillingStatementVersionStatement(v)
	if err != nil {
		return err
	}
	if integrity.Rows < 0 || integrity.Gross != statement.Summary.GrossQuota || integrity.Refund != statement.Summary.RefundQuota {
		return errors.New("statement integrity mismatch")
	}
	var artifacts []BillingStatementArtifact
	if err := tx.WithContext(ctx).Where("version_id = ?", v.ID).Find(&artifacts).Error; err != nil {
		return err
	}
	summary, detail := false, false
	for _, artifact := range artifacts {
		if artifact.Staged || artifact.Sha256 == "" {
			return errors.New("statement artifacts are incomplete")
		}
		if artifact.Role == "summary_csv" {
			summary = true
		}
		if artifact.Role == "detail_csv" && artifact.LineCount == integrity.Rows {
			detail = true
		}
	}
	if !summary || !detail {
		return errors.New("statement artifacts are incomplete")
	}
	if statement.DataQuality != nil && statement.DataQuality.Status != "complete" {
		var ack struct {
			Acknowledged  bool   `json:"acknowledged"`
			DraftPublicId string `json:"draft_public_id"`
		}
		if common.UnmarshalJsonStr(acknowledged, &ack) != nil || !ack.Acknowledged || ack.DraftPublicId != v.DraftPublicId {
			return errors.New("acknowledge this draft's data quality before confirmation")
		}
	}
	return nil
}
