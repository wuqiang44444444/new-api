package model

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
)

// 任务失败事件（仅观察）：在各正式失败终态提交出口登记，不决定退款、重发、
// 换渠道或任务终态。判定来源是条件更新/CAS 的真实迁移结果，不由日志层重判。
// 去重依赖既有状态条件更新：只有本次提交者且旧状态与目标失败状态不同才发事件。

// submitTaskFailureEvent 在通用/图片/Batch 任务失败终态真实提交后登记事件。
// fromStatus 为迁移前快照状态；批量等无快照路径传空以跳过同状态保护。
func submitTaskFailureEvent(task *Task, fromStatus TaskStatus, stage string) {
	if task == nil {
		return
	}
	if task.Status != TaskStatusFailure && task.Status != TaskStatusExpired {
		return
	}
	if fromStatus != "" && fromStatus == task.Status {
		return
	}
	reason := "task_failed"
	if task.Status == TaskStatusExpired {
		reason = "task_expired"
	}
	detail := map[string]string{"platform": string(task.Platform)}
	if task.FailReason != "" {
		detail["fail_reason"] = task.PublicFailReason()
	}
	if task.PrivateData.UpstreamRequestID != "" {
		// 创建时冻结的上游请求 ID；不得宣称是失败轮询当次的上游请求 ID。
		detail["create_upstream_request_id"] = task.PrivateData.UpstreamRequestID
	}
	// 创建请求关联 ID：事件身份与 CH 排序键依赖该列，尽量不为空。
	requestID := ""
	if task.PrivateData.Execution != nil {
		requestID = task.PrivateData.Execution.RequestID
	}
	clienterrlog.SubmitBackendEvent(clienterrlog.BackendEvent{
		EventType: clienterrlog.EventTaskFailure,
		Module:    ErrorEventModuleRelay,
		Stage:     stage,
		Reason:    reason,
		Model:     task.Properties.OriginModelName,
		ChannelID: task.ChannelId,
		UserID:    task.UserId,
		TaskID:    task.TaskID,
		RequestID: requestID,
		Status:    0,
		Detail:    detail,
	})
}

// submitMidjourneyFailureEvent 在 Midjourney 失败终态真实提交后登记事件。
func submitMidjourneyFailureEvent(midjourney *Midjourney, fromStatus string) {
	if midjourney == nil || midjourney.Status != "FAILURE" || fromStatus == "FAILURE" {
		return
	}
	detail := map[string]string{"platform": "midjourney"}
	if midjourney.Action != "" {
		detail["action"] = midjourney.Action
	}
	if midjourney.FailReason != "" {
		detail["fail_reason"] = common.PublicTaskErrorMessageForModel(midjourney.FailReason, "", "")
	}
	clienterrlog.SubmitBackendEvent(clienterrlog.BackendEvent{
		EventType: clienterrlog.EventTaskFailure,
		Module:    ErrorEventModuleRelay,
		Stage:     "midjourney",
		Reason:    "task_failed",
		ChannelID: midjourney.ChannelId,
		UserID:    midjourney.UserId,
		TaskID:    midjourney.MjId,
		Status:    0,
		Detail:    detail,
	})
}
