package service

import (
	"github.com/gin-gonic/gin"
)

// CaptureImageClientResponse owns only the observation wrapper. The original
// Gin writer remains authoritative for status, bytes written and SSE flushing.
func CaptureImageClientResponse(c *gin.Context, capture *ImageErrorEvidence) func() {
	if capture == nil {
		return func() {}
	}
	c.Request = c.Request.WithContext(WithImageErrorEvidence(c.Request.Context(), capture))
	original := c.Writer
	c.Writer = &imageEvidenceWriter{ResponseWriter: original, capture: capture}
	return func() { c.Writer = original; capture.FinishClient(original.Status(), original.Header()) }
}

type imageEvidenceWriter struct {
	gin.ResponseWriter
	capture *ImageErrorEvidence
}

func (w *imageEvidenceWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.capture.CaptureClientBody(p[:n])
	w.capture.recordError("client_write", err)
	return n, err
}
func (w *imageEvidenceWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	w.capture.CaptureClientBody([]byte(s[:n]))
	w.capture.recordError("client_write", err)
	return n, err
}
