package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// GetTaskForProtocol is the legacy user-scoped lookup used by callers that do
// not have an application/token scope. The protocol is required so a lookup
// cannot silently broaden to another task contract.
func GetTaskForProtocol(userID int, taskID, protocol string, includeDeleted bool) (*Task, bool, error) {
	if userID <= 0 || strings.TrimSpace(taskID) == "" || strings.TrimSpace(protocol) == "" {
		return nil, false, nil
	}
	query := DB.Where(
		"user_id = ? AND task_id = ? AND client_protocol = ?",
		userID,
		strings.TrimSpace(taskID),
		protocol,
	)
	if !includeDeleted {
		query = query.Where("client_deleted_at = ?", 0)
	}
	var task Task
	err := query.First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}
