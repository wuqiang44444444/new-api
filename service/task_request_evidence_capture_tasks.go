package service

import (
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// AttachTaskRequestEvidenceTask 在任务行原子落库后关联证据与任务/attempt。
// Best-effort：失败只告警，不影响任务与资金事实。
func AttachTaskRequestEvidenceTask(c *gin.Context, task *model.Task) {
	session := evidenceSessionFrom(c)
	if session == nil || session.evidenceID <= 0 || task == nil {
		return
	}
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	model.AttachTaskRequestEvidenceTask(session.evidenceID, task.TaskID, attemptID)
	model.UpsertTaskRequestEvidenceUpstreamRequestID(session.evidenceID, task.PrivateData.UpstreamRequestID)
}

// CaptureTaskPollingEvidence 记录一次轮询出站与响应（adapter 解析前正文）。
// 传输错误也记录。找不到证据索引时惰性创建最小索引，不伪造北向正文。
func CaptureTaskPollingEvidence(
	task *model.Task,
	method, target string,
	statusCode int,
	requestHeaders http.Header,
	responseBody []byte,
	pollErr error,
) {
	if !evidenceEnabled() || task == nil {
		return
	}
	evidence, exists, err := model.FindTaskRequestEvidenceByTaskID(task.TaskID)
	if err != nil {
		common.SysError("evidence polling lookup failed: " + err.Error())
		return
	}
	if !exists {
		now := common.GetTimestamp()
		evidence = &model.TaskRequestEvidence{
			TaskID:      task.TaskID,
			UserID:      task.UserId,
			AppID:       task.PrivateData.AppID,
			Kind:        model.TaskRequestEvidenceKindVideoTask,
			ChannelID:   task.ChannelId,
			CreatedAt:   now,
			RetainUntil: evidenceRetentionUntil(system_setting.GetTaskRequestEvidenceConfig(), now),
		}
		if err := model.CreateTaskRequestEvidence(evidence); err != nil {
			common.SysError("evidence polling create failed: " + err.Error())
			return
		}
	}
	if parsed, err := url.Parse(target); err == nil {
		target = evidenceTargetURL(&http.Request{URL: parsed})
	} else {
		target = ""
	}
	phase := model.TaskRequestEvidencePhaseResponded
	if pollErr != nil {
		phase = model.TaskRequestEvidencePhaseFailed
	}
	event := &model.TaskRequestEvidenceEvent{
		EvidenceId:  evidence.Id,
		Stage:       model.TaskRequestEvidenceStagePolling,
		Phase:       phase,
		StatusCode:  statusCode,
		Method:      method,
		Target:      target,
		ByteCount:   int64(len(responseBody)),
		Complete:    pollErr == nil,
		ContentType: "application/json",
		Redacted:    true,
		Detail: evidenceMarshalDetail(map[string]any{
			"error": errorString(pollErr),
		}),
		CreatedAt: common.GetTimestamp(),
	}
	if err := persistTaskEvidenceBody(event, responseBody); err != nil {
		common.SysError("evidence polling event unavailable")
	}
}

// IsTaskRequestEvidenceEnabled 报告证据采集是否启用。
func IsTaskRequestEvidenceEnabled() bool {
	return evidenceEnabled()
}

// CaptureTaskContentDeliveryEvidence 记录一次结果代理交付：
// 状态码、实际写出字节与取消/网络错误。正文媒体不入证据库。
func CaptureTaskContentDeliveryEvidence(taskID string, statusCode int, bytesWritten int64, deliverErr error) {
	if !evidenceEnabled() || taskID == "" {
		return
	}
	task, exists, err := model.GetByOnlyTaskId(taskID)
	if err != nil || !exists || task == nil {
		return
	}
	evidence, evidenceExists, lookupErr := model.FindTaskRequestEvidenceByTaskID(taskID)
	if lookupErr != nil {
		return
	}
	if !evidenceExists {
		now := common.GetTimestamp()
		evidence = &model.TaskRequestEvidence{
			TaskID:      taskID,
			UserID:      task.UserId,
			AppID:       task.PrivateData.AppID,
			Kind:        model.TaskRequestEvidenceKindVideoTask,
			ChannelID:   task.ChannelId,
			CreatedAt:   now,
			RetainUntil: evidenceRetentionUntil(system_setting.GetTaskRequestEvidenceConfig(), now),
		}
		if err := model.CreateTaskRequestEvidence(evidence); err != nil {
			common.SysError("evidence delivery create failed: " + err.Error())
			return
		}
	}
	phase := model.TaskRequestEvidencePhaseCompleted
	if deliverErr != nil {
		phase = model.TaskRequestEvidencePhaseFailed
	}
	event := &model.TaskRequestEvidenceEvent{
		EvidenceId: evidence.Id,
		Stage:      model.TaskRequestEvidenceStageClientDelivery,
		Phase:      phase,
		StatusCode: statusCode,
		ByteCount:  bytesWritten,
		Complete:   deliverErr == nil,
		Redacted:   false,
		Detail: evidenceMarshalDetail(map[string]any{
			"bytes_written": bytesWritten,
			"error":         errorString(deliverErr),
		}),
		CreatedAt: common.GetTimestamp(),
	}
	if err := model.CreateTaskRequestEvidenceEvent(event); err != nil {
		common.SysError("evidence delivery event failed: " + err.Error())
	}
}

// TaskRequestEvidenceKindForTaskRelay 按北向入口判定证据类别。
func TaskRequestEvidenceKindForTaskRelay(c *gin.Context) string {
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == constant.TaskPlatformSuno {
		return model.TaskRequestEvidenceKindAudioTask
	}
	return model.TaskRequestEvidenceKindVideoTask
}
