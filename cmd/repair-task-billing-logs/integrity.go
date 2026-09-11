package main

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// This is a read-only comparison of a closed task lifecycle with its log
// projection. Anonymous historical creates can prove only aggregate equality,
// not ownership of an individual task's initial charge.
type taskLogIntegrity struct {
	Status                                                     string
	SettledQuota, NetDifference                                int64
	MissingInitialLogs, UnlinkedInitialLogs, UnreconciledTasks int
}

func inspectTaskLogIntegrity(tasks map[string]*model.Task, logs []model.Log, s scope) taskLogIntegrity {
	result := taskLogIntegrity{Status: "matched"}
	incomplete := len(tasks) == 0
	for _, task := range tasks {
		async := task.PrivateData.AsyncBilling
		if async == nil || async.State != model.TaskBillingStateSettled || async.TargetQuota == nil || *async.TargetQuota != task.Quota {
			incomplete = true
		}
		result.SettledQuota += int64(task.Quota)
	}
	creates := make(map[string]int)
	nets := make(map[string]int64)
	totalCreates, net := 0, int64(0)
	for _, log := range logs {
		inside := log.CreatedAt >= s.Start && log.CreatedAt <= s.End
		var other map[string]any
		if common.UnmarshalJsonStr(log.Other, &other) != nil {
			if inside {
				incomplete = true
			}
			continue
		}
		taskID, _ := other["task_id"].(string)
		if !inside {
			if tasks[taskID] != nil {
				incomplete = true
			}
			continue
		}
		if log.Quota < 0 {
			incomplete = true
		}
		quota := int64(log.Quota)
		if log.Type == model.LogTypeRefund {
			quota = -quota
		}
		net += quota
		create := isInitialTaskLog(log, other)
		if create {
			totalCreates++
			creates[taskID]++
		}
		if taskID == "" && create {
			result.UnlinkedInitialLogs++
			continue
		}
		if task := tasks[taskID]; task == nil || log.ChannelId != task.ChannelId {
			incomplete = true
			continue
		}
		nets[taskID] += quota
	}
	result.NetDifference = result.SettledQuota - net
	result.MissingInitialLogs = max(len(tasks)-totalCreates, 0)
	for id, task := range tasks {
		if creates[id] > 1 || (creates[id] == 1 && nets[id] != int64(task.Quota)) {
			result.UnreconciledTasks++
		}
	}
	switch {
	case incomplete:
		result.Status = "incomplete"
	case result.NetDifference != 0 || totalCreates != len(tasks) || result.UnreconciledTasks > 0:
		result.Status = "mismatch"
	case result.UnlinkedInitialLogs > 0:
		result.Status = "aggregate_matched"
	}
	return result
}

func isInitialTaskLog(log model.Log, other map[string]any) bool {
	return log.Type == model.LogTypeConsume && (other["is_task"] == true || other["task_billing_event"] == "create") &&
		other["actual_quota"] == nil && other["pre_consumed_quota"] == nil && other["task_billing_event"] != "adjustment"
}

func finalizeRepairIntegrity(report *repairReport, tasks map[string]*model.Task, logs []model.Log, s scope, apply bool) error {
	report.After = inspectTaskLogIntegrity(tasks, logs, s)
	if apply && (report.After.Status == "mismatch" || report.After.Status == "incomplete") {
		return fmt.Errorf("task/log integrity is %s (net quota difference %d); repair rolled back", report.After.Status, report.After.NetDifference)
	}
	return nil
}
