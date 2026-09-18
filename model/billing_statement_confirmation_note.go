package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
)

// StageBillingStatementConfirmationNote serializes manifest creation with the
// immutable version. Uploads happen outside this transaction and may safely
// repeat the same frozen bytes at the same key after an uncertain result.
func StageBillingStatementConfirmationNote(ctx context.Context, artifact *BillingStatementArtifact) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var v BillingStatementVersion
		if err := lockForUpdate(tx).First(&v, artifact.VersionId).Error; err != nil {
			return err
		}
		if v.Status != BillingStatementVersionConfirmed || artifact.Role != "confirmation_note" {
			return ErrBillingStatementVersionConflict
		}
		var existing BillingStatementArtifact
		err := tx.Where("version_id = ? AND role = ?", v.ID, artifact.Role).First(&existing).Error
		if err == nil {
			*artifact = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(artifact).Error
	})
}
