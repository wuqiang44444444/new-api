package model

import "gorm.io/gorm"

func (t *Task) BeforeCreate(_ *gorm.DB) error {
	if t.AppID <= 0 {
		t.AppID = t.PrivateData.AppID
	}
	if t.AppID <= 0 {
		t.AppID = t.PrivateData.TokenId
	}
	if t.PrivateData.AppID <= 0 && t.AppID > 0 {
		t.PrivateData.AppID = t.AppID
	}
	return nil
}

// GetByTaskIDForApp returns a task only when it belongs to the authenticated
// API application. API-facing task operations must not fall back to user-wide
// visibility because one account can own multiple independent applications.
func GetByTaskIDForApp(userID, appID int, taskID string) (*Task, bool, error) {
	if userID <= 0 || appID <= 0 || taskID == "" {
		return nil, false, nil
	}
	var task Task
	err := DB.Where("user_id = ? AND app_id = ? AND task_id = ?", userID, appID, taskID).
		First(&task).Error
	exists, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return &task, exists, nil
}
