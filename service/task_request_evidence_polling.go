package service

import (
	"bytes"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"net/http"
)

func CaptureFallbackTaskPollingEvidence(adaptor any, task *model.Task, method, target string, status int, headers http.Header, body []byte, err error) {
	if _, captures := adaptor.(interface{ CapturesRawPollingEvidence() }); captures {
		return
	}
	CaptureTaskPollingEvidence(task, method, target, status, headers, body, err)
}

// Called at the adapter's HTTP boundary, before parsing or normalizing the reply.
func AttachRawTaskPollingEvidence(task *model.Task, req *http.Request, resp *http.Response, transportErr error) {
	if !evidenceEnabled() || task == nil {
		return
	}
	if transportErr != nil || resp == nil {
		CaptureTaskPollingEvidence(task, req.Method, evidenceTargetURL(req), 0, req.Header, nil, transportErr)
		return
	}
	evidence, exists, err := model.FindTaskRequestEvidenceByTaskID(task.TaskID)
	if err != nil {
		common.SysError("evidence polling index unavailable")
		return
	}
	if !exists {
		now := common.GetTimestamp()
		evidence = &model.TaskRequestEvidence{TaskID: task.TaskID, UserID: task.UserId, AppID: task.PrivateData.AppID, Kind: model.TaskRequestEvidenceKindVideoTask, CreatedAt: now, RetainUntil: evidenceRetentionUntil(system_setting.GetTaskRequestEvidenceConfig(), now)}
		if err := model.CreateTaskRequestEvidence(evidence); err != nil {
			common.SysError("evidence polling index unavailable")
			return
		}
	}
	if resp.Body == nil {
		return
	}
	resp.Body = &evidenceResponseBodyTee{session: &taskRequestEvidenceSession{evidenceID: evidence.Id}, origin: resp.Body, buffer: bytes.NewBuffer(nil), maxBytes: system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes, stage: model.TaskRequestEvidenceStagePolling, statusCode: resp.StatusCode, method: req.Method, target: evidenceTargetURL(req), contentType: resp.Header.Get("Content-Type"), headers: resp.Header.Clone()}
}

// Some batch adapters synthesize responses without an HTTP request descriptor.
func CaptureTaskPollingHTTPResponse(task *model.Task, baseURL string, resp *http.Response, body []byte, readErr error) {
	method, target, status := http.MethodGet, baseURL, 0
	var headers http.Header
	if resp != nil {
		status = resp.StatusCode
		if resp.Request != nil {
			method = resp.Request.Method
			target = evidenceTargetURL(resp.Request)
			headers = resp.Request.Header
		}
	}
	CaptureTaskPollingEvidence(task, method, target, status, headers, body, readErr)
}
