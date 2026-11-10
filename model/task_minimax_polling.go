package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

// IsMiniMaxJDTask identifies a task by its frozen platform and protocol.
func (t *Task) IsMiniMaxJDTask() bool {
	return t != nil && t.Platform == constant.TaskPlatform(MiniMaxLinkTaskPlatform()) &&
		t.PrivateData.VideoUpstreamProtocol == dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1)
}

// DeferMiniMaxTaskQuery updates only the schedule using the current private
// data, so a failed query cannot overwrite a concurrent observation or billing.
func DeferMiniMaxTaskQuery(id int64, nextAt int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := lockForUpdate(tx).Select("id", "private_data").First(&task, id).Error; err != nil {
			return err
		}
		if task.PrivateData.VideoUpstreamNextQueryAt >= nextAt {
			return nil
		}
		task.PrivateData.VideoUpstreamNextQueryAt = nextAt
		return tx.Model(&task).UpdateColumn("private_data", task.PrivateData).Error
	})
}
