package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// CompleteQueuedAssetOperationJobTx cancels a watchdog that has not been
// claimed. A running owner remains responsible for finishing its own lease.
func CompleteQueuedAssetOperationJobTx(tx *gorm.DB, idempotencyKey string) error {
	return tx.Model(&AssetOperationJob{}).
		Where("idempotency_key = ? AND status IN ?", idempotencyKey, []string{AssetJobPending, AssetJobFailed}).
		Updates(map[string]any{
			"status": AssetJobSucceeded, "locked_by": "", "locked_until": int64(0),
			"last_error": "", "updated_at": common.GetTimestamp(),
		}).Error
}

func AssetOperationJobUnresolved(idempotencyKey string) (bool, error) {
	var count int64
	err := DB.Model(&AssetOperationJob{}).
		Where("idempotency_key = ? AND status IN ?", idempotencyKey, []string{AssetJobPending, AssetJobFailed, AssetJobRunning}).
		Count(&count).Error
	return count != 0, err
}
