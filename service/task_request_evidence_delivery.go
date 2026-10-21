package service

import (
	"bytes"
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"io"
	"strings"
)

// Embed Gin's writer so Flush, Hijack, CloseNotify and status handling retain
// their native behavior. Capture only bytes accepted by the downstream writer.
type evidenceClientWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	writeErr error
	finished bool
}

func (w *evidenceClientWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err != nil {
		w.writeErr = err
	} else if n != len(p) {
		w.writeErr = io.ErrShortWrite
	}
	limit := system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes
	take := min(int64(n), max(int64(0), limit-int64(w.body.Len())))
	w.body.Write(p[:int(take)])
	return n, err
}
func (w *evidenceClientWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Deferred at the outer response presenter, after any client error JSON is sent.
func FinishTaskRequestEvidenceClientDelivery(c *gin.Context) {
	session := evidenceSessionFrom(c)
	if session == nil {
		return
	}
	if c.GetBool(taskCreateEvidenceDeliveryKey) {
		return
	}
	writer, ok := c.Writer.(*evidenceClientWriter)
	if !ok || writer.finished {
		return
	}
	writer.finished = true
	contentType := writer.Header().Get("Content-Type")
	event := &model.TaskRequestEvidenceEvent{EvidenceId: session.evidenceID, Stage: model.TaskRequestEvidenceStageClientDelivery, StatusCode: writer.Status(), ContentType: contentType, CreatedAt: common.GetTimestamp()}
	if strings.Contains(contentType, "json") || strings.HasPrefix(contentType, "text/") {
		persistRawEvidenceEvent(event, writer.Header(), nil, writer.body.Bytes(), errorString(writer.writeErr))
	} else if err := model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()); err != nil && !errors.Is(err, model.ErrTaskRequestEvidenceBodyClosed) {
		evidencePersistWarning("delivery event", event.Stage, err)
	}
	writer.body.Reset()
}

var _ io.Writer = (*evidenceClientWriter)(nil)
