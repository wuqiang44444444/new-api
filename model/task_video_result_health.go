package model

import "context"

// CountVideoResultTransferBacklog includes rows in backoff and active leases.
func CountVideoResultTransferBacklog(ctx context.Context) (int64, error) {
	var pending int64
	err := DB.WithContext(ctx).Model(&TaskVideoResultObject{}).Where("status IN ?", []string{
		TaskVideoResultStatusPending, TaskVideoResultStatusTransferring,
	}).Count(&pending).Error
	return pending, err
}
