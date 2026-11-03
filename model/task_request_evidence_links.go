package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Only already authenticated task lists from polling can create these links.
// The original response is stored once, not copied for each task in a batch.
type EvidenceTaskLink struct {
	EvidenceID int64  `gorm:"primaryKey;autoIncrement:false"`
	TaskID     string `gorm:"primaryKey;type:varchar(191);index:idx_evidence_task_link_task"`
	UserID     int    `gorm:"primaryKey;autoIncrement:false"`
	AppID      int    `gorm:"primaryKey;autoIncrement:false"`
}

func AttachEvidenceTasks(evidenceID int64, tasks []EvidenceTaskLink, retentionSeconds int64) error {
	if len(tasks) == 0 {
		return nil
	}
	links := make([]EvidenceTaskLink, 0, len(tasks))
	for _, task := range tasks {
		if task.TaskID == "" || task.UserID <= 0 {
			continue
		}
		task.EvidenceID = evidenceID
		links = append(links, task)
	}
	if len(links) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := writableEvidenceIndex(tx, evidenceID, common.GetTimestamp(), retentionSeconds); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error
	})
}
