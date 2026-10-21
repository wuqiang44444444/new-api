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
	if !evidenceEnabled() || task == nil || task.TaskID == "" {
		return
	}
	if resp == nil {
		CaptureTaskPollingEvidence(task, req.Method, evidenceTargetURL(req), 0, req.Header, nil, transportErr)
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

	tee := &evidenceResponseBodyTee{session: &taskRequestEvidenceSession{evidenceID: evidence.Id}, origin: resp.Body, buffer: bytes.NewBuffer(nil), maxBytes: system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes, stage: model.TaskRequestEvidenceStagePolling, statusCode: resp.StatusCode, method: req.Method, target: evidenceTargetURL(req), contentType: resp.Header.Get("Content-Type"), headers: resp.Header.Clone(), transportErr: transportErr}
	if req.URL != nil {
		tee.requestQuery = req.URL.Query()
	}
	if resp.Body == nil {
		tee.origin = http.NoBody
		_ = tee.Close()
		return
	}
	resp.Body = tee
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

func CaptureFallbackTaskPollingHTTPResponse(adaptor any, task *model.Task, target string, resp *http.Response, body []byte, err error) {
	if _, captures := adaptor.(interface{ CapturesRawPollingEvidence() }); captures {
		return
	}
	CaptureTaskPollingHTTPResponse(task, target, resp, body, err)
}
