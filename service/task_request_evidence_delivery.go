package service

import (
	"bytes"
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
	body      bytes.Buffer
	written   int64
	failed    bool
	truncated bool
	finished  bool
}

func (w *evidenceClientWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.written += int64(n)
	if err != nil || n != len(p) {
		w.failed = true
	}
	limit := system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes
	take := min(int64(n), max(int64(0), limit-int64(w.body.Len())))
	w.body.Write(p[:int(take)])
	if take < int64(n) {
		w.truncated = true
	}
	return n, err
}
func (w *evidenceClientWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Deferred at the outer response presenter, after any client error JSON is sent.
func FinishTaskRequestEvidenceClientDelivery(c *gin.Context) {
	session := evidenceSessionFrom(c)
	if session == nil {
		return
	}
	writer, ok := c.Writer.(*evidenceClientWriter)
	if !ok || writer.finished {
		return
	}
	writer.finished = true
	complete := !writer.failed && !writer.truncated && c.Request.Context().Err() == nil
	phase := model.TaskRequestEvidencePhaseCompleted
	if !complete {
		phase = model.TaskRequestEvidencePhaseFailed
	}
	if writer.truncated {
		phase = model.TaskRequestEvidencePhaseTruncated
	}
	contentType := writer.Header().Get("Content-Type")
	event := &model.TaskRequestEvidenceEvent{EvidenceId: session.evidenceID, Stage: model.TaskRequestEvidenceStageClientDelivery, Phase: phase, StatusCode: writer.Status(), ContentType: contentType, ByteCount: writer.written, Complete: complete, CreatedAt: common.GetTimestamp()}
	if strings.Contains(contentType, "json") || strings.HasPrefix(contentType, "text/") {
		if err := persistTaskEvidenceBody(event, writer.body.Bytes()); err != nil {
			common.SysError("evidence client response unavailable")
		}
	} else {
		// Binary delivery statistics supplement the upstream body; avoid storing a
		// duplicate media object or claiming that bytes reached the user's device.
		event.Complete = !writer.failed && c.Request.Context().Err() == nil
		if event.Complete {
			event.Phase = model.TaskRequestEvidencePhaseCompleted
		}
		event.Detail = evidenceMarshalDetail(map[string]any{"media_body_archived": false})
		if err := model.CreateTaskRequestEvidenceEvent(event); err != nil {
			common.SysError("evidence client delivery unavailable")
		}
	}
	writer.body.Reset()
}

var _ io.Writer = (*evidenceClientWriter)(nil)
