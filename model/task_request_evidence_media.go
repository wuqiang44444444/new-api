package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// FindTaskMediaEvidence reads the eligibility frozen by a generic media create.
// It cannot turn historical full-site evidence into media eligibility.
func FindTaskMediaEvidence(taskID string, userID, appID int) (*TaskRequestEvidence, error) {
	var index TaskRequestEvidence
	err := DB.Where("task_id = ? AND user_id = ? AND app_id = ? AND kind IN ?", taskID, userID, appID, []string{"image_request", TaskRequestEvidenceKindVideoTask, TaskRequestEvidenceKindAudioTask, "media_task"}).Order("id").First(&index).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &index, err
}

// MJ SubmitTime is mutable provider data. Only a newly observed creation can
// establish an anchor; queries reuse the existing index or skip capture.
func GetMidjourneyMediaEvidence(seed TaskRequestEvidence, taskID string, now, retentionSeconds int64) (*TaskRequestEvidence, error) {
	var index TaskRequestEvidence
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Native TokenId is a billing refund marker, not an application owner.
		// Application isolation is provided by the creation evidence index below.
		query := tx.Model(&Midjourney{}).Where("user_id = ? AND mj_id = ?", seed.UserID, taskID)
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			if err := query.UpdateColumn("progress", gorm.Expr("progress")).Error; err != nil {
				return err
			}
		}
		var task Midjourney
		if err := lockForUpdate(query).First(&task).Error; err != nil {
			return err
		}
		err := tx.Where("task_id = ? AND user_id = ? AND app_id = ? AND client_protocol = ?", taskID, seed.UserID, seed.AppID, "midjourney").Order("id").First(&index).Error
		if err == nil {
			_, err = writableEvidenceIndex(tx, index.Id, now, retentionSeconds)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if seed.CreatedAt <= 0 || seed.CreatedAt > now {
			return ErrTaskRequestEvidenceCreationTime
		}
		if seed.CreatedAt+retentionSeconds < now {
			return ErrTaskRequestEvidenceBodyClosed
		}
		index = seed
		index.TaskID = taskID
		index.ChannelID = task.ChannelId
		index.Kind = "image_request"
		index.ClientProtocol = "midjourney"
		index.UpdatedAt = now
		index.RetainUntil = seed.CreatedAt + retentionSeconds
		return tx.Create(&index).Error
	})
	return &index, err
}
