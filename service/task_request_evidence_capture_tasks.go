package service

import (
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func AttachTaskRequestEvidenceTask(c *gin.Context, task *model.Task) {
	if task == nil || c == nil || c.Request == nil {
		return
	}
	capture := apiEvidenceFrom(c.Request.Context())
	if capture == nil {
		return
	}
	capture.mu.Lock()
	capture.Index.TaskID = task.TaskID
	capture.Index.UserID = task.UserId
	capture.Index.AppID = task.AppID
	capture.Index.AttemptID = int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
	capture.Index.UpstreamRequestID = task.PrivateData.UpstreamRequestID
	capture.mu.Unlock()
}
func CaptureTaskPollingEvidence(task *model.Task, method, target string, status int, headers http.Header, body []byte, pollErr error) {
	if !evidenceEnabled() || task == nil {
		return
	}
	capture := newAPIRequestEvidence(evidenceIndexForTask(task))
	parsed, err := url.Parse(target)
	if err != nil {
		return
	}
	request := &http.Request{Method: method, URL: parsed}
	record := capture.newRecord(model.TaskRequestEvidenceStagePolling, EvidenceObjectMeta{Method: method, Target: evidenceTargetURL(request), RawQuery: parsed.RawQuery, StatusCode: status, Headers: headers, ContentType: headers.Get("Content-Type"), Error: errorString(pollErr)}, evidenceFileBody("", parsed.Path, headers))
	record.append(body)
	capture.Finish(false)
}
func IsTaskRequestEvidenceEnabled() bool                        { return evidenceEnabled() }
func TaskRequestEvidenceKindForTaskRelay(c *gin.Context) string { return mediaEvidenceContextKind(c) }
