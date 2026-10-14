package model

const SystemTaskTypeTaskRequestEvidenceMonitor = "task_request_evidence_monitor"

type EvidenceCleanupHealth struct {
	CheckedAt               int64    `json:"checked_at"`
	LastSucceededAt         int64    `json:"last_succeeded_at"`
	MonitoringSince         int64    `json:"monitoring_since"`
	ConsecutiveFailures     int      `json:"consecutive_failures"`
	PendingDatabaseIndexes  int64    `json:"pending_database_indexes"`
	OldestDatabasePendingAt int64    `json:"oldest_database_pending_at"`
	PendingIndexes          int64    `json:"pending_indexes"`
	OldestPendingAt         int64    `json:"oldest_pending_at"`
	FreeBytes               *uint64  `json:"free_bytes,omitempty"`
	FreeInodes              *uint64  `json:"free_inodes,omitempty"`
	Alerts                  []string `json:"alerts"`
}

func GetEvidenceCleanupHealth(now, retentionSeconds int64) (*EvidenceCleanupHealth, error) {
	health := &EvidenceCleanupHealth{CheckedAt: now, MonitoringSince: now, Alerts: []string{}}
	var first SystemTask
	if err := DB.Where("type = ?", SystemTaskTypeTaskRequestEvidenceRetention).Order("id").Limit(1).Find(&first).Error; err != nil {
		return nil, err
	}
	if first.ID > 0 {
		health.MonitoringSince = first.CreatedAt
	}
	var success SystemTask
	if err := DB.Where("type = ? AND status = ?", SystemTaskTypeTaskRequestEvidenceRetention, SystemTaskStatusSucceeded).Order("id desc").Limit(1).Find(&success).Error; err != nil {
		return nil, err
	}
	health.LastSucceededAt = success.UpdatedAt
	var recent []*SystemTask
	if err := DB.Where("type = ? AND status IN ?", SystemTaskTypeTaskRequestEvidenceRetention, []SystemTaskStatus{SystemTaskStatusSucceeded, SystemTaskStatusFailed}).Order("id desc").Limit(2).Find(&recent).Error; err != nil {
		return nil, err
	}
	for _, task := range recent {
		if task.Status != SystemTaskStatusFailed {
			break
		}
		health.ConsecutiveFailures++
	}
	query := taskRequestEvidenceCleanupCandidates(now, retentionSeconds, 0)
	if err := query.Count(&health.PendingIndexes).Error; err != nil {
		return nil, err
	}
	var oldest TaskRequestEvidence
	if err := query.Order("created_at").Limit(1).Find(&oldest).Error; err != nil {
		return nil, err
	}
	health.OldestPendingAt = oldest.CreatedAt
	databaseQuery := DB.Model(&TaskRequestEvidence{}).Where("body_expired = ? AND (created_at < ? OR EXISTS (SELECT 1 FROM task_request_evidence_events WHERE evidence_id = task_request_evidences.id))", true, now-EvidenceIndexRetentionSeconds)
	if err := databaseQuery.Count(&health.PendingDatabaseIndexes).Error; err != nil {
		return nil, err
	}
	var oldestDatabase TaskRequestEvidence
	if err := databaseQuery.Order("created_at").Limit(1).Find(&oldestDatabase).Error; err != nil {
		return nil, err
	}
	health.OldestDatabasePendingAt = oldestDatabase.CreatedAt
	return health, nil
}
