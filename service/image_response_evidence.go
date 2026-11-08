package service

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// WriteOriginalImageResponse forwards the unmodified result of a successful
// io.ReadAll on src.Body. Only a direct evidence writer sees the same bytes.
func WriteOriginalImageResponse(c *gin.Context, src *http.Response, data []byte) {
	var writer *imageEvidenceWriter
	switch w := c.Writer.(type) {
	case *apiEvidenceWriter:
		writer = &w.imageEvidenceWriter
	case *imageEvidenceWriter:
		writer = w
	}
	if writer != nil && !writer.Written() && src != nil && len(data) > 0 {
		if body, ok := src.Body.(*imageEvidenceBody); ok && body.capture == writer.capture {
			writer.originalImageBody = body.record.sealImageResponseSource()
			defer func() { writer.originalImageBody = nil }()
		}
	}
	IOCopyBytesGracefully(c, src, data)
}

func (r *imageEvidenceExchange) sealImageResponseSource() *evidencePiece {
	if r == nil || r.stage != model.TaskRequestEvidenceStageUpstreamResponse || r.skip {
		return nil
	}
	// Close the observation after EOF, before a delivery reference can be
	// queued on the same worker. The handler still owns the transport body.
	r.close()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.split || r.stopped || r.piece == nil || r.piece.aborted.Load() {
		return nil
	}
	return r.piece
}
