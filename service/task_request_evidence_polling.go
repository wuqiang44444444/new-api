package service

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
)

func CaptureFallbackTaskPollingEvidence(adaptor any, task *model.Task, method, target string, status int, headers http.Header, body []byte, err error) {
	if _, captures := adaptor.(interface{ CapturesRawPollingEvidence() }); captures {
		return
	}
	CaptureTaskPollingEvidence(task, method, target, status, headers, body, err)
}
func AttachRawTaskPollingEvidence(task *model.Task, req *http.Request, resp *http.Response, transportErr error) {
	if !evidenceEnabled() || task == nil {
		return
	}
	if resp != nil {
		if _, ok := resp.Body.(*imageEvidenceBody); ok {
			return
		}
	}
	capture := newAPIRequestEvidence(evidenceIndexForTask(task))
	capture.observeResponse(req, resp, transportErr, model.TaskRequestEvidenceStagePolling)
	if resp != nil {
		if body, ok := resp.Body.(*imageEvidenceBody); ok {
			body.finish = func() { capture.Finish(false) }
		}
	} else {
		capture.Finish(false)
	}
}
func CaptureTaskPollingHTTPResponse(task *model.Task, target string, resp *http.Response, body []byte, err error) {
	method, status := http.MethodGet, 0
	var headers http.Header
	if resp != nil {
		status = resp.StatusCode
		headers = resp.Header
		if resp.Request != nil {
			method = resp.Request.Method
			target = evidenceTargetURL(resp.Request)
		}
	}
	CaptureTaskPollingEvidence(task, method, target, status, headers, body, err)
}
func CaptureFallbackTaskPollingHTTPResponse(adaptor any, task *model.Task, target string, resp *http.Response, body []byte, err error) {
	if _, captures := adaptor.(interface{ CapturesRawPollingEvidence() }); captures {
		return
	}
	CaptureTaskPollingHTTPResponse(task, target, resp, body, err)
}
