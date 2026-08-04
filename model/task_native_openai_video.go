package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// GetNativeOpenAIVideoTaskForApp also recognizes rc23 tasks created before
// client_protocol existed. The fallback remains application-scoped and rejects
// registered Link SKUs so it cannot bridge the two public contracts.
func GetNativeOpenAIVideoTaskForApp(userID, appID int, taskID string) (*Task, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if userID <= 0 || taskID == "" {
		return nil, false, nil
	}
	var task *Task
	var exists bool
	var err error
	if appID > 0 {
		task, exists, err = GetVideoTaskForProtocol(userID, appID, taskID, TaskClientProtocolOpenAIVideos, false)
	} else {
		task, exists, err = GetTaskForProtocol(userID, taskID, TaskClientProtocolOpenAIVideos, false)
	}
	if err != nil || exists {
		return task, exists, err
	}
	var legacy Task
	err = DB.Where(
		"user_id = ? AND task_id = ? AND client_protocol = ? AND client_deleted_at = ?",
		userID,
		taskID,
		"",
		0,
	).First(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	legacyAppID := legacy.AppID
	if legacyAppID <= 0 {
		legacyAppID = legacy.PrivateData.AppID
	}
	if legacyAppID <= 0 {
		legacyAppID = legacy.PrivateData.TokenId
	}
	if appID > 0 && legacyAppID != appID {
		return nil, false, nil
	}
	modelName := strings.TrimSpace(legacy.Properties.OriginModelName)
	if modelName == "" && legacy.PrivateData.BillingContext != nil {
		modelName = strings.TrimSpace(legacy.PrivateData.BillingContext.OriginModelName)
	}
	if modelName == "" || IsRegisteredLinkSKU(modelName) {
		return nil, false, nil
	}
	return &legacy, true, nil
}
