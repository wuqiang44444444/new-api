package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

const taskRequestEvidenceContextKey = "task_request_evidence_session"

var ErrTaskRequestEvidenceUnavailable = errors.New("task sending permission unavailable")

func IsTaskRequestEvidenceUnavailable(err error) bool {
	return errors.Is(err, ErrTaskRequestEvidenceUnavailable)
}
func evidenceEnabled() bool { return system_setting.GetTaskRequestEvidenceConfig().Enabled }

type taskRequestEvidenceSession struct {
	capture    *ImageErrorEvidence
	standalone bool
}

func evidenceSessionFrom(c *gin.Context) *taskRequestEvidenceSession {
	if c == nil {
		return nil
	}
	value, _ := c.Get(taskRequestEvidenceContextKey)
	session, _ := value.(*taskRequestEvidenceSession)
	return session
}

func BeginTaskRequestEvidence(c *gin.Context, kind string) error {
	if !evidenceEnabled() || evidenceDiskPressure.Load() || c == nil || c.Request == nil || evidenceSessionFrom(c) != nil {
		return nil
	}
	capture := apiEvidenceFrom(c.Request.Context())
	standalone := capture == nil
	if standalone {
		if mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path) == "" {
			return nil
		}
		capture = newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: c.GetString(common.RequestIdKey), Kind: kind})
		c.Request = c.Request.WithContext(WithImageErrorEvidence(c.Request.Context(), capture))
		record := capture.requestRecord(c.Request, model.TaskRequestEvidenceStageNorthReceive)
		if c.Request.Body != nil {
			c.Request.Body = &imageEvidenceBody{ReadCloser: c.Request.Body, capture: capture, record: record}
		}
		c.Writer = &imageEvidenceWriter{ResponseWriter: c.Writer, capture: capture, requestPath: c.Request.URL.Path}
	}
	updateRequestEvidenceIdentity(c, capture)
	c.Set(taskRequestEvidenceContextKey, &taskRequestEvidenceSession{capture: capture, standalone: standalone})
	return nil
}
func EvidenceObjectKey(evidenceID, eventID int64) string {
	return fmt.Sprintf("%d/%d.bin", evidenceID, eventID)
}

// The independent durable permission check must survive any observation changes.
func CaptureSouthboundTaskRequestEvidence(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	BindAPIRequestEvidence(c, req, info)
	return verifyEvidenceAttemptBeforeSend(c, info)
}
func evidenceTargetURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	target := *req.URL
	target.RawQuery = ""
	target.User = nil
	return target.String()
}
func AttachTaskRequestEvidenceUpstreamResponse(c *gin.Context, resp *http.Response) {
	if c == nil || c.Request == nil || resp == nil {
		return
	}
	capture := apiEvidenceFrom(c.Request.Context())
	if capture == nil {
		return
	}
	capture.mu.Lock()
	capture.Index.UpstreamRequestID = c.GetString(common.UpstreamRequestIdKey)
	capture.mu.Unlock()
	// The shared transport already owns this exact response. Independently
	// constructed clients still join the same canonical observer here.
	if _, ok := resp.Body.(*imageEvidenceBody); !ok {
		capture.observeResponse(resp.Request, resp, nil, model.TaskRequestEvidenceStageUpstreamResponse)
	}
}
func CaptureTaskEvidenceTransportFailure(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	capture := apiEvidenceFrom(c.Request.Context())
	if capture != nil {
		capture.recordError(model.TaskRequestEvidenceStageTransport, errors.New("transport_error"))
	}
}
