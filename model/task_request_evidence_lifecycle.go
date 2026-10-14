package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const EvidenceIndexRetentionSeconds int64 = 7 * 86400

var ErrTaskRequestEvidenceCreationTime = errors.New("evidence task creation time unavailable")

// The lock is shared by event reservation, object publication and cleanup.
func writableEvidenceIndex(tx *gorm.DB, id, now, retentionSeconds int64) (*TaskRequestEvidence, error) {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := tx.Model(&TaskRequestEvidence{}).Where("id = ?", id).UpdateColumn("updated_at", gorm.Expr("updated_at")).Error; err != nil {
			return nil, err
		}
	}
	var index TaskRequestEvidence
	if err := lockForUpdate(tx).First(&index, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskRequestEvidenceBodyClosed
		}
		return nil, err
	}
	deadline := index.CreatedAt + retentionSeconds
	if index.RetainUntil > 0 && index.RetainUntil < deadline {
		deadline = index.RetainUntil
	}
	if index.BodyExpired || index.CleanupRequestedAt > 0 || deadline < now {
		return nil, ErrTaskRequestEvidenceBodyClosed
	}
	return &index, nil
}

func GetOrCreateTaskMediaEvidence(task *Task, kind string, now, retentionSeconds int64) (*TaskRequestEvidence, error) {
	var index TaskRequestEvidence
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock the authoritative Task even when it has no evidence index yet,
		// so simultaneous pollers cannot create parallel indexes.
		query := tx.Model(&Task{}).Where("task_id = ? AND user_id = ? AND app_id = ?", task.TaskID, task.UserId, task.AppID)
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			if err := query.UpdateColumn("updated_at", gorm.Expr("updated_at")).Error; err != nil {
				return err
			}
		}
		var durable Task
		if err := lockForUpdate(query).First(&durable).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskRequestEvidenceBodyClosed
			}
			return err
		}
		if durable.CreatedAt <= 0 || durable.CreatedAt > now {
			return ErrTaskRequestEvidenceCreationTime
		}
		if durable.CreatedAt+retentionSeconds < now {
			return ErrTaskRequestEvidenceBodyClosed
		}
		err := tx.Where("task_id = ? AND user_id = ? AND app_id = ?", task.TaskID, task.UserId, task.AppID).Order("id").First(&index).Error
		if err == nil {
			_, err = writableEvidenceIndex(tx, index.Id, now, retentionSeconds)
			if err != nil {
				return err
			}
			deadline := durable.CreatedAt + retentionSeconds
			if index.RetainUntil <= 0 || index.RetainUntil > deadline {
				if err := tx.Model(&TaskRequestEvidence{}).Where("id = ?", index.Id).UpdateColumn("retain_until", deadline).Error; err != nil {
					return err
				}
				index.RetainUntil = deadline
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		index = TaskRequestEvidence{TaskID: durable.TaskID, UserID: durable.UserId, AppID: durable.AppID, TokenID: durable.PrivateData.TokenId, ChannelID: durable.ChannelId, Kind: kind, ClientProtocol: durable.ClientProtocol, ClientModel: durable.Properties.OriginModelName, CreatedAt: durable.CreatedAt, UpdatedAt: now, RetainUntil: durable.CreatedAt + retentionSeconds}
		return tx.Create(&index).Error
	})
	return &index, err
}

func TaskRequestEvidenceEventBatch(evidenceID, afterID int64, limit int) ([]*TaskRequestEvidenceEvent, error) {
	var events []*TaskRequestEvidenceEvent
	err := DB.Select("id", "evidence_id").Where("evidence_id = ? AND id > ?", evidenceID, afterID).Order("id").Limit(limit).Find(&events).Error
	return events, err
}

// Includes completed body cleanups: a crash between object and row deletion
// leaves these indexes as the durable database recovery queue.
func TaskRequestEvidenceDatabaseCandidates(now, target, afterID int64, limit int) ([]*TaskRequestEvidence, error) {
	var indexes []*TaskRequestEvidence
	query := DB.Where("body_expired = ? AND id > ?", true, afterID)
	if target > 0 {
		query = query.Where("created_at < ?", target)
	} else {
		query = query.Where("created_at < ? OR EXISTS (SELECT 1 FROM task_request_evidence_events WHERE evidence_id = task_request_evidences.id)", now-EvidenceIndexRetentionSeconds)
	}
	err := query.Order("id").Limit(limit).Find(&indexes).Error
	return indexes, err
}

func DeleteClosedEvidenceEventBatch(evidenceID int64, limit int) (int64, error) {
	var deleted int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			if err := tx.Model(&TaskRequestEvidence{}).Where("id = ?", evidenceID).UpdateColumn("updated_at", gorm.Expr("updated_at")).Error; err != nil {
				return err
			}
		}
		var index TaskRequestEvidence
		if err := lockForUpdate(tx).First(&index, "id = ?", evidenceID).Error; err != nil {
			return err
		}
		if !index.BodyExpired || index.CleanupRequestedAt <= 0 {
			return ErrTaskRequestEvidenceBodyClosed
		}
		var ids []int64
		if err := tx.Model(&TaskRequestEvidenceEvent{}).Where("evidence_id = ?", evidenceID).Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		result := tx.Where("evidence_id = ? AND id IN ?", evidenceID, ids).Delete(&TaskRequestEvidenceEvent{})
		deleted = result.RowsAffected
		return result.Error
	})
	return deleted, err
}

func DeleteExpiredEvidenceIndex(evidenceID, now int64) (int64, error) {
	// A closed index cannot acquire events; reservation shares its row lock.
	result := DB.Where("id = ? AND body_expired = ? AND cleanup_requested_at > 0 AND created_at < ? AND NOT EXISTS (SELECT 1 FROM task_request_evidence_events WHERE evidence_id = task_request_evidences.id) AND NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.task_id = task_request_evidences.task_id AND tasks.user_id = task_request_evidences.user_id AND tasks.app_id = task_request_evidences.app_id AND tasks.created_at >= ?)", evidenceID, true, now-EvidenceIndexRetentionSeconds, now-EvidenceIndexRetentionSeconds).Delete(&TaskRequestEvidence{})
	return result.RowsAffected, result.Error
}

func DeleteExpiredEvidenceAccessBatch(now int64, limit int) (int64, error) {
	var ids []int64
	if err := DB.Model(&TaskRequestEvidenceAccessLog{}).Where("created_at < ?", now-EvidenceIndexRetentionSeconds).Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Where("id IN ?", ids).Delete(&TaskRequestEvidenceAccessLog{})
	return result.RowsAffected, result.Error
}

func DeleteExpiredEvidenceTaskHistoryBatch(now int64, limit int) (int64, error) {
	types := []string{SystemTaskTypeTaskRequestEvidenceRetention, SystemTaskTypeTaskRequestEvidenceMonitor}
	var latest []int64
	if err := DB.Model(&SystemTask{}).Where("type IN ?", types).Select("MAX(id)").Group("type").Pluck("MAX(id)", &latest).Error; err != nil {
		return 0, err
	}
	if len(latest) == 0 {
		return 0, nil
	}
	var ids []int64
	if err := DB.Model(&SystemTask{}).Where("type IN ? AND status IN ? AND updated_at < ? AND id NOT IN ?", types, []SystemTaskStatus{SystemTaskStatusSucceeded, SystemTaskStatusFailed}, now-30*86400, latest).Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Where("id IN ? AND status IN ?", ids, []SystemTaskStatus{SystemTaskStatusSucceeded, SystemTaskStatusFailed}).Delete(&SystemTask{})
	return result.RowsAffected, result.Error
}

func OrphanTaskRequestEvidenceIDs(cutoff, afterID int64, limit int) ([]int64, error) {
	var ids []int64
	err := DB.Model(&TaskRequestEvidenceEvent{}).Where("evidence_id > ? AND NOT EXISTS (SELECT 1 FROM task_request_evidences WHERE task_request_evidences.id = task_request_evidence_events.evidence_id)", afterID).Group("evidence_id").Having("MAX(created_at) < ?", cutoff).Order("evidence_id").Limit(limit).Pluck("evidence_id", &ids).Error
	return ids, err
}

func DeleteOrphanTaskRequestEvidenceEventBatch(evidenceID int64, limit int) (int64, error) {
	var ids []int64
	if err := DB.Model(&TaskRequestEvidenceEvent{}).Where("evidence_id = ?", evidenceID).Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Where("id IN ? AND evidence_id = ? AND NOT EXISTS (SELECT 1 FROM task_request_evidences WHERE task_request_evidences.id = task_request_evidence_events.evidence_id)", ids, evidenceID).Delete(&TaskRequestEvidenceEvent{})
	return result.RowsAffected, result.Error
}
