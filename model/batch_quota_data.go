package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// The job's settled acknowledgement and its additive dashboard projection
// share a transaction; a split-log-DB retry cannot count its lines twice.
func recordBatchQuotaDataTx(tx *gorm.DB, job *BatchJob, task *Task, quota, prompt, completion int) error {
	if !common.DataExportEnabled {
		return nil
	}
	var user User
	if err := tx.Select("id", "username").First(&user, task.UserId).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	return tx.Create(&QuotaData{UserID: task.UserId, Username: user.Username, ModelName: task.Properties.OriginModelName,
		CreatedAt: now - now%3600, UseGroup: task.Group, TokenID: task.PrivateData.TokenId, ChannelID: task.ChannelId, NodeName: task.PrivateData.NodeName,
		Count: common.QuotaFromFloat(float64(job.LineCount)), Quota: quota, TokenUsed: common.QuotaFromFloat(float64(prompt) + float64(completion)),
	}).Error
}
