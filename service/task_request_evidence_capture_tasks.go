package service

import (
	"errors"
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
	if task != nil {
		if capture := apiEvidenceFrom(c.Request.Context()); capture != nil {
			capture.mu.Lock()
			capture.Index.TaskID = task.TaskID
			capture.Index.UpstreamRequestID = task.PrivateData.UpstreamRequestID
			capture.mu.Unlock()
		}
	}
	session := evidenceSessionFrom(c)
	if session == nil || session.evidenceID <= 0 || task == nil {
		return
	}
	attemptID := int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	model.AttachTaskRequestEvidenceTask(session.evidenceID, task.TaskID, attemptID, task.UserId, task.AppID)
	model.SetTaskRequestEvidenceUpstreamRequestID(session.evidenceID, task.PrivateData.UpstreamRequestID)
}

// CaptureTaskPollingEvidence 记录一次轮询出站与响应（adapter 解析前正文）。
// 传输错误也记录。找不到证据索引时惰性创建最小索引，不伪造北向正文。
// 采集由冻结 Task 的媒体类型驱动；已到期索引不追加正文也不续期。
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
	evidence, err := taskMediaEvidenceIndex(task)
	if err != nil {
		common.SysError("evidence polling index unavailable")
		return
	}
	if evidence == nil {
		return
	}

	if parsed, err := url.Parse(target); err == nil {
		target = evidenceTargetURL(&http.Request{URL: parsed})
	} else {
		target = ""
	}
	event := &model.TaskRequestEvidenceEvent{EvidenceId: evidence.Id, Stage: model.TaskRequestEvidenceStagePolling, StatusCode: statusCode, Method: method, Target: target, ContentType: "application/json", CreatedAt: common.GetTimestamp()}
	persistRawEvidenceEvent(event, requestHeaders, nil, responseBody[:min(int64(len(responseBody)), max(int64(0), system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes))], errorString(pollErr))
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
	evidence, lookupErr := taskMediaEvidenceIndex(task)
	if lookupErr != nil || evidence == nil {
		return
	}

	event := &model.TaskRequestEvidenceEvent{EvidenceId: evidence.Id, Stage: model.TaskRequestEvidenceStageClientDelivery, StatusCode: statusCode, CreatedAt: common.GetTimestamp()}
	if err := model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()); err != nil && !errors.Is(err, model.ErrTaskRequestEvidenceBodyClosed) {
		evidencePersistWarning("delivery event", event.Stage, err)
	}
}

// TaskRequestEvidenceKindForTaskRelay 按北向入口判定证据类别。
func TaskRequestEvidenceKindForTaskRelay(c *gin.Context) string {
	return mediaEvidenceContextKind(c)
}
