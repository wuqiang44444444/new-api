package model

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

func (t *Task) BeforeCreate(_ *gorm.DB) error {
	t.AppID = t.applicationID()
	if t.PrivateData.AppID <= 0 && t.AppID > 0 {
		t.PrivateData.AppID = t.AppID
	}
	return nil
}

func (t *Task) applicationID() int {
	if t.AppID > 0 {
		return t.AppID
	}
	if t.PrivateData.AppID > 0 {
		return t.PrivateData.AppID
	}
	return t.PrivateData.TokenId
}

// GetByTaskIDForApp returns a task only when it belongs to the authenticated
// API application. API-facing task operations must not fall back to user-wide
// visibility because one account can own multiple independent applications.
func GetByTaskIDForApp(userID, appID int, taskID string) (*Task, bool, error) {
	return getTaskForApp(DB, userID, appID, taskID, "", true)
}

func getTaskForApp(db *gorm.DB, userID, appID int, taskID, protocol string, includeDeleted bool) (*Task, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if userID <= 0 || appID <= 0 || taskID == "" {
		return nil, false, nil
	}
	query := taskApplicationCandidates(db, userID, appID).Where("task_id = ?", taskID)
	if protocol != "" {
		query = query.Where("client_protocol = ?", protocol)
	}
	if !includeDeleted {
		query = query.Where("client_deleted_at = ?", 0)
	}
	var tasks []Task
	if err := query.Order("id").Find(&tasks).Error; err != nil {
		return nil, false, err
	}
	var legacy *Task
	for i := range tasks {
		task := &tasks[i]
		if task.AppID == appID {
			return task, true, nil
		}
		if legacy == nil && task.applicationID() == appID {
			legacy = task
		}
	}
	return legacy, legacy != nil, nil
}

// Legacy rows can prove ownership with the application or Key frozen at
// creation. Positive app_id values remain authoritative.
func taskApplicationCandidates(db *gorm.DB, userID, appID int) *gorm.DB {
	return db.Where("user_id = ? AND (app_id IN ? OR app_id IS NULL)", userID, []int{0, appID})
}

func GetAppTasksByIDsForPlatforms(userID, appID int, platforms []constant.TaskPlatform, taskIDs []string) ([]*Task, error) {
	if userID <= 0 || appID <= 0 || len(platforms) == 0 || len(taskIDs) == 0 {
		return nil, nil
	}
	var tasks []*Task
	if err := taskApplicationCandidates(DB, userID, appID).
		Where("platform IN ? AND task_id IN ?", platforms, taskIDs).Find(&tasks).Error; err != nil {
		return nil, err
	}
	// Older native tasks without a client protocol were outside the app_id
	// migration. Their frozen application or token must still match.
	return slices.DeleteFunc(tasks, func(task *Task) bool { return task.applicationID() != appID }), nil
}
