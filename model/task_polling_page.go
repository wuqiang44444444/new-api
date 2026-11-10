package model

import (
	"context"

	"github.com/QuantumNous/new-api/constant"
)

// ListUnfinishedSyncTaskPage selects shared polling candidates after a local Task ID.
func ListUnfinishedSyncTaskPage(ctx context.Context, afterID int64, limit int) ([]*Task, error) {
	var tasks []*Task
	err := DB.WithContext(ctx).
		Where("id > ?", afterID).
		Where("progress != ?", "100%").
		Where("status NOT IN ?", TerminalTaskStatuses()).
		Where("client_protocol IS NULL OR client_protocol <> ?", TaskClientProtocolImageOpenAIV1).
		Where("platform <> ?", constant.TaskPlatformAzureBatch).
		Order("id").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}
