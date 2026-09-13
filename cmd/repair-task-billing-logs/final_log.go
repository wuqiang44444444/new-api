package main

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"gorm.io/gorm"
)

// Only an explicitly selected closed lifecycle with one linked initial log
// can acquire a missing completion projection. Funding and counters are untouched.
func repairMissingFinalLog(tx *gorm.DB, tasks map[string]*model.Task, logs []model.Log, s scope, apply bool, report *repairReport) ([]model.Log, error) {
	if s.FinalTaskID == 0 {
		return logs, nil
	}
	var task *model.Task
	for _, candidate := range tasks {
		if candidate.ID == s.FinalTaskID {
			task = candidate
			break
		}
	}
	if task == nil {
		return logs, fmt.Errorf("requested completion task is outside repair scope")
	}
	async := task.PrivateData.AsyncBilling
	if async == nil || async.State != model.TaskBillingStateSettled || async.TargetQuota == nil || *async.TargetQuota != task.Quota || task.Status != model.TaskStatusSuccess {
		return logs, fmt.Errorf("completion repair requires a successfully settled task")
	}
	if tx.Migrator().HasTable(&model.TaskBillingDelivery{}) {
		var count int64
		if err := tx.Model(&model.TaskBillingDelivery{}).Where("task_row_id = ?", task.ID).Count(&count).Error; err != nil {
			return logs, err
		}
		if count > 0 {
			return logs, fmt.Errorf("task has durable delivery records; use normal log recovery")
		}
	}
	var initial *model.Log
	finals := 0
	for i := range logs {
		row := &logs[i]
		var other map[string]any
		if err := common.UnmarshalJsonStr(row.Other, &other); err != nil {
			return logs, err
		}
		id, _ := other["task_id"].(string)
		if id == "" && isInitialTaskLog(*row, other) && row.ChannelId == task.ChannelId {
			return logs, fmt.Errorf("unlinked initial log prevents completion repair")
		}
		if id != task.TaskID {
			continue
		}
		if row.CreatedAt < s.Start || row.CreatedAt > s.End {
			return logs, fmt.Errorf("completion lifecycle extends outside scope")
		}
		if isInitialTaskLog(*row, other) {
			if initial != nil {
				return logs, fmt.Errorf("duplicate initial logs prevent completion repair")
			}
			initial = row
		} else {
			finals++
		}
	}
	if finals == 1 {
		return logs, nil
	}
	if finals > 1 || initial == nil || initial.Quota < 0 {
		return logs, fmt.Errorf("completion repair requires exactly one linked initial log and no final logs")
	}
	if task.FinishTime < s.Start || task.FinishTime > s.End {
		return logs, fmt.Errorf("trusted completion time is outside repair scope")
	}
	result, _, err := service.ComputeTaskTieredBilling(task)
	if err != nil || result.Clamp != nil || result.ActualQuotaAfterGroup != task.Quota {
		return logs, fmt.Errorf("frozen usage does not reproduce settled quota")
	}
	row, err := service.BuildTaskBillingDeliveryLog(task, model.TaskBillingDelivery{Event: "adjustment", BeforeQuota: initial.Quota, AfterQuota: task.Quota, CompletionTokens: async.ActualTokens, UsageReported: async.ActualUsageReported})
	if err != nil {
		return logs, err
	}
	row.CreatedAt = task.FinishTime
	row.Username, row.TokenName = initial.Username, initial.TokenName
	row.RequestId = fmt.Sprintf("billing-log-repair:%d:final", task.ID)
	row.Content = "Completion log restored from frozen settlement; no funding changes"
	if apply {
		if err := tx.Create(row).Error; err != nil {
			return logs, err
		}
	}
	report.FinalLogs++
	report.NetAfter += int64(task.Quota) - int64(initial.Quota)
	return append(logs, *row), nil
}
