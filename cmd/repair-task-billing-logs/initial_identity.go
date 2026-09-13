package main

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// Historical creates can be linked only by an exact frozen Provider request ID,
// with the same customer, key, channel and a uniquely transferred creation hold.
// Time proximity and equal amounts never establish ownership.
func linkInitialTaskLogs(tx *gorm.DB, tasks map[string]*model.Task, logs []model.Log, s scope, apply bool, report *repairReport) ([]model.Log, error) {
	for i := range logs {
		row := &logs[i]
		var other map[string]any
		if err := common.UnmarshalJsonStr(row.Other, &other); err != nil {
			return logs, fmt.Errorf("invalid metadata at log %d", row.Id)
		}
		taskID, validTaskID := other["task_id"].(string)
		if other["task_id"] != nil && !validTaskID {
			return logs, fmt.Errorf("invalid task identity at log %d", row.Id)
		}
		if !isInitialTaskLog(*row, other) || taskID != "" || row.UpstreamRequestId == "" {
			continue
		}
		var matched *model.Task
		for _, task := range tasks {
			if task.UserId != row.UserId || task.PrivateData.TokenId != row.TokenId || task.ChannelId != row.ChannelId || task.Properties.OriginModelName != row.ModelName || task.PrivateData.UpstreamRequestID != row.UpstreamRequestId {
				continue
			}
			if matched != nil {
				return logs, fmt.Errorf("ambiguous Provider request identity at log %d", row.Id)
			}
			matched = task
		}
		if matched == nil {
			continue
		}
		if row.CreatedAt < s.Start || row.CreatedAt > s.End {
			return logs, fmt.Errorf("linked initial log is outside the selected period")
		}
		if tx.Migrator().HasTable(&model.TaskBillingDelivery{}) {
			var count int64
			if err := tx.Model(&model.TaskBillingDelivery{}).Where("task_row_id = ?", matched.ID).Count(&count).Error; err != nil {
				return logs, err
			}
			if count > 0 {
				return logs, fmt.Errorf("task has durable delivery records; use normal log recovery")
			}
		}
		var attempts []model.TaskCreateAttempt
		if err := tx.Select("id", "held_quota", "status", "billing_hold_state", "upstream_request_id").Where("public_task_id = ? AND user_id = ? AND token_id = ? AND app_id = ? AND channel_id = ? AND public_model = ?", matched.TaskID, matched.UserId, matched.PrivateData.TokenId, matched.AppID, matched.ChannelId, matched.Properties.OriginModelName).Find(&attempts).Error; err != nil {
			return logs, err
		}
		if len(attempts) != 1 || attempts[0].Status != model.TaskCreateAttemptComplete || attempts[0].BillingHoldState != model.TaskCreateAttemptBillingTransferred || attempts[0].HeldQuota != row.Quota || attempts[0].UpstreamRequestID != row.UpstreamRequestId {
			return logs, fmt.Errorf("initial log %d lacks a matching transferred hold", row.Id)
		}
		other["task_id"] = matched.TaskID
		admin, _ := other["admin_info"].(map[string]any)
		if admin == nil {
			admin = map[string]any{}
			other["admin_info"] = admin
		}
		admin["billing_statement_identity_repair"] = map[string]any{"version": 1, "task_row_id": matched.ID, "attempt_row_id": attempts[0].ID, "at": time.Now().Unix(), "reason": "exact_frozen_provider_request_id"}
		encoded, err := common.Marshal(other)
		if err != nil {
			return logs, err
		}
		if apply {
			result := tx.Model(&model.Log{}).Where("id = ? AND other = ?", row.Id, row.Other).Update("other", string(encoded))
			if result.Error != nil {
				return logs, result.Error
			}
			if result.RowsAffected != 1 {
				return logs, fmt.Errorf("initial log changed concurrently")
			}
		}
		row.Other = string(encoded)
		report.LinkedInitialLogs++
	}
	return logs, nil
}
