package service

import (
	"io"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func CaptureImageClientResponse(c *gin.Context, capture *ImageErrorEvidence) func() {
	if capture == nil {
		return func() {}
	}
	c.Request = c.Request.WithContext(WithImageErrorEvidence(c.Request.Context(), capture))
	original := c.Writer
	writer := &imageEvidenceWriter{ResponseWriter: original, capture: capture, requestPath: c.Request.URL.Path}
	c.Writer = writer
	return func() {
		updateRequestEvidenceIdentity(c, capture)
		writer.prepareDelivery()
		c.Writer = original
		capture.Finish(false)
	}
}

type imageEvidenceWriter struct {
	gin.ResponseWriter
	capture           *ImageErrorEvidence
	observeIdentity   func()
	requestPath       string
	originalImageBody *evidencePiece
	sharedDelivery    bool
}

func (w *imageEvidenceWriter) prepareDelivery() {
	w.capture.mu.Lock()
	exists := w.capture.deliveryPrepared
	w.capture.deliveryPrepared = true
	w.capture.mu.Unlock()
	if exists {
		return
	}
	if w.observeIdentity != nil {
		w.observeIdentity()
	}
	meta := EvidenceObjectMeta{StatusCode: w.Status(), Headers: w.Header(), ContentType: w.Header().Get("Content-Type")}
	record := w.capture.newRecord(model.TaskRequestEvidenceStageClientDelivery, meta, evidenceFileBody("", w.requestPath, w.Header()))
	w.capture.mu.Lock()
	w.capture.delivery = record
	w.capture.mu.Unlock()
}
func (w *imageEvidenceWriter) Write(p []byte) (int, error) {
	source := w.originalImageBody
	w.originalImageBody = nil
	n, err := w.ResponseWriter.Write(p)
	if w.sharedDelivery {
		// A later write belongs to its own delivery event. The original event
		// already describes the full image write and has been sealed.
		w.capture.mu.Lock()
		w.capture.deliveryPrepared = false
		w.capture.mu.Unlock()
		w.sharedDelivery = false
	}
	w.prepareDelivery()
	observedErr := err
	if observedErr == nil && n != len(p) {
		observedErr = io.ErrShortWrite
	}
	w.capture.mu.Lock()
	record := w.capture.delivery
	w.capture.mu.Unlock()
	if source != nil && observedErr == nil && record.seal(source) {
		w.sharedDelivery = true
		return n, err
	}
	if record != nil && observedErr != nil {
		record.mu.Lock()
		record.meta.Error = observedErr.Error()
		record.mu.Unlock()
	}
	w.capture.CaptureClientBody(p[:n])
	if observedErr != nil {
		w.capture.recordError("client_write", observedErr)
	}
	return n, err
}
func (w *imageEvidenceWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
