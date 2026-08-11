package model

// GetUnrefundedFailedTasks returns failed tasks whose non-zero quota marks a
// pending refund. Legacy timeout tasks are excluded before LIMIT is applied.
func GetUnrefundedFailedTasks(updatedBefore int64, limit int) []*Task {
	if limit <= 0 {
		return nil
	}

	var tasks []*Task
	err := DB.Where("status = ?", TaskStatusFailure).
		Where("quota != ?", 0).
		Where("updated_at <= ?", updatedBefore).
		Where("(submit_time <= ? OR submit_time >= ?)", 0, TaskRefundLegacyCutoff).
		Order("id").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasTaskPollingWork reports whether polling has unfinished work or a failed
// task that still needs refund reconciliation.
func HasTaskPollingWork() bool {
	if HasUnfinishedSyncTasks() {
		return true
	}
	if HasTaskCreateAttemptWork(GetDBTimestamp()) {
		return true
	}

	var id int64
	err := DB.Model(&Task{}).
		Where("status = ?", TaskStatusFailure).
		Where("quota != ?", 0).
		Where("(submit_time <= ? OR submit_time >= ?)", 0, TaskRefundLegacyCutoff).
		Limit(1).
		Pluck("id", &id).Error
	if err == nil && id != 0 {
		return true
	}
	return HasTerminalTasksPendingBilling()
}
