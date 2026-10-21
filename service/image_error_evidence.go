package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Image error evidence is observation only. A bounded memory reservation covers
// both in-flight captures and queued writes. Read/Write only observe bytes.
// Existing bounded image captures queue their writes; all-request captures persist at
// handler completion. Evidence failures never change the business outcome.
var imageEvidenceSlots = make(chan struct{}, 8)
var imageEvidenceQueue = make(chan *ImageErrorEvidence, 8)
var imageEvidenceAccepted, imageEvidenceWritten, imageEvidenceFailed, imageEvidenceDropped atomic.Uint64
var imageEvidenceSkipped atomic.Uint64

type imageEvidenceContextKey struct{}

type ImageErrorEvidence struct {
	Index             model.TaskRequestEvidence
	mu                sync.Mutex
	remaining         int64
	records           []*imageEvidenceExchange
	responseRecords   map[*http.Response]*imageEvidenceExchange
	allRequests       bool
	deliveryStatsOnly bool
	finished          bool
}

type imageEvidenceExchange struct {
	CreatedAt   int64
	Query       url.Values
	Stage       string
	Method      string
	Target      string
	Status      int
	Headers     http.Header
	ContentType string
	Error       string
	Body        bytes.Buffer
	EOF         bool // Stops bounded post-rejection draining; never persisted as completeness.
	taskOwned   bool
}

func init() {
	go func() {
		for capture := range imageEvidenceQueue {
			writeImageErrorEvidence(capture)
		}
	}()
}

func NewImageErrorEvidence(index model.TaskRequestEvidence) *ImageErrorEvidence {
	config := system_setting.GetTaskRequestEvidenceConfig()
	if !config.Enabled {
		return nil
	}
	if config.MaxResponseBytes <= 0 {
		imageEvidenceFailed.Add(1)
		return nil
	}
	select {
	case imageEvidenceSlots <- struct{}{}:
	default:
		imageEvidenceDropped.Add(1)
		return nil
	}
	if index.Kind == "" {
		index.Kind = "image_request"
	}
	return &ImageErrorEvidence{Index: index, remaining: config.MaxResponseBytes}
}

func NewImageTaskErrorEvidence(task *model.Task) *ImageErrorEvidence {
	index, err := taskMediaEvidenceIndex(task)
	if err != nil || index == nil {
		return nil
	}
	return NewImageErrorEvidence(*index)
}

func WithImageErrorEvidence(ctx context.Context, capture *ImageErrorEvidence) context.Context {
	if capture == nil {
		return ctx
	}
	return context.WithValue(ctx, imageEvidenceContextKey{}, capture)
}

func ImageErrorEvidenceFrom(ctx context.Context) *ImageErrorEvidence {
	capture, _ := ctx.Value(imageEvidenceContextKey{}).(*ImageErrorEvidence)
	return capture
}

// ObserveImageHTTPExchange keeps the exact consumed response before adapters
// parse or normalize it. Capture does not interpret success or failure bodies.
func ObserveImageHTTPExchange(ctx context.Context, req *http.Request, resp *http.Response, err error, stage string) {
	capture := ImageErrorEvidenceFrom(ctx)
	if capture == nil {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.finished {
		return
	}
	record := &imageEvidenceExchange{Stage: stage, CreatedAt: common.GetTimestamp()}
	if req != nil {
		record.Method = req.Method
		record.Target = evidenceTargetURL(req)
	}
	if err != nil {
		record.Error = err.Error()
	}
	if resp != nil {
		record.Status = resp.StatusCode
		record.Headers = resp.Header.Clone()
		record.ContentType = resp.Header.Get("Content-Type")
		if resp.Body != nil {
			resp.Body = &imageEvidenceBody{ReadCloser: resp.Body, capture: capture, record: record}
		}
	}
	capture.records = append(capture.records, record)
}

// RecordImageDeliveryError preserves a storage/DB cause privately; ordinary
// diagnostics continue to use only platform-defined stages and numeric facts.
func RecordImageDeliveryError(ctx context.Context, stage string, err error) {
	if err == nil {
		return
	}
	capture := ImageErrorEvidenceFrom(ctx)
	if capture == nil {
		capture = apiEvidenceFrom(ctx)
	}
	if capture == nil {
		return
	}
	capture.recordError(stage, err)
}

func (capture *ImageErrorEvidence) recordError(stage string, err error) {
	if err == nil {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.finished {
		return
	}
	if capture.allRequests && stage == "client_write" {
		for _, record := range capture.records {
			if record.Stage == "client_delivery" {
				record.Error = "write_error"
			}
		}
	}
	capture.records = append(capture.records, &imageEvidenceExchange{Stage: stage, CreatedAt: common.GetTimestamp(), Error: err.Error()})
}

type imageEvidenceBody struct {
	io.ReadCloser
	capture *ImageErrorEvidence
	record  *imageEvidenceExchange
}

func (b *imageEvidenceBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.capture.mu.Lock()
	if b.capture.finished {
		b.capture.mu.Unlock()
		return n, err
	}
	b.capture.appendBody(b.record, p[:n])
	if err == io.EOF {
		b.record.EOF = true
	}
	if err != nil && err != io.EOF {
		b.record.Error = err.Error()
	}
	b.capture.mu.Unlock()
	return n, err
}

func (b *imageEvidenceBody) Close() error {
	err := b.ReadCloser.Close()
	if err != nil {
		b.capture.mu.Lock()
		if !b.capture.finished {
			b.record.Error = err.Error()
		}
		b.capture.mu.Unlock()
	}
	return err
}

func (c *ImageErrorEvidence) appendBody(record *imageEvidenceExchange, p []byte) {
	keep := int64(len(p))
	if c.allRequests {
		keep = min(keep, max(int64(0), evidenceMaxBodyBytes()-int64(record.Body.Len())))
	}
	if keep > c.remaining {
		keep = c.remaining
	}
	if keep > 0 {
		_, _ = record.Body.Write(p[:keep])
		c.remaining -= keep
	}
}

// CaptureClientBody is called after the real writer, and records exactly the
// bytes it accepted. Flush/Hijack/etc remain delegated by the Gin wrapper.
func (c *ImageErrorEvidence) CaptureClientBody(p []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		return
	}
	var record *imageEvidenceExchange
	for _, candidate := range c.records {
		if candidate.Stage == "client_delivery" {
			record = candidate
			break
		}
	}
	if record == nil {
		record = &imageEvidenceExchange{Stage: "client_delivery", CreatedAt: common.GetTimestamp()}
		c.records = append(c.records, record)
	}
	c.appendBody(record, p)
}
func (c *ImageErrorEvidence) FinishClient(status int, headers http.Header) {
	if c == nil {
		return
	}
	c.mu.Lock()
	found := false
	for _, r := range c.records {
		if r.Stage == "client_delivery" {
			found = true
			r.Status = status
			r.Headers = headers.Clone()
			r.ContentType = headers.Get("Content-Type")
		}
	}
	if !found && (status >= 400 || c.allRequests) {
		c.records = append(c.records, &imageEvidenceExchange{Stage: "client_delivery", CreatedAt: common.GetTimestamp(), Status: status, Headers: headers.Clone(), ContentType: headers.Get("Content-Type")})
	}
	c.mu.Unlock()
	c.Finish(status >= 400)
}

// Abandon 丢弃本次观察记录而不落盘：任务证据会话已持有同一交换的
// 阶段证据时使用，保证每次实际 HTTP 交换只保留一份证据。
func (c *ImageErrorEvidence) Abandon() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finished = true
	c.records = nil
}

func (c *ImageErrorEvidence) Finish(_ bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		return
	}
	c.finished = true
	if c.allRequests {
		defer func() {
			if recover() != nil {
				imageEvidenceFailed.Add(1)
				common.SysError("API request evidence persistence panicked")
			}
		}()
		imageEvidenceAccepted.Add(1)
		if err := persistAPIRequestEvidence(c); err != nil {
			imageEvidenceFailed.Add(1)
			common.SysError("API request evidence persistence failed")
		} else {
			imageEvidenceWritten.Add(1)
		}
		return
	}
	// There is one queue slot for every memory reservation; never wait on I/O.
	select {
	case imageEvidenceQueue <- c:
		imageEvidenceAccepted.Add(1)
	default:
		imageEvidenceDropped.Add(1)
		<-imageEvidenceSlots
	}
}

func writeImageErrorEvidence(c *ImageErrorEvidence) {
	defer func() {
		if recover() != nil {
			imageEvidenceFailed.Add(1)
		}
		<-imageEvidenceSlots
	}()
	if len(c.records) == 0 {
		imageEvidenceSkipped.Add(1)
		return
	}
	if err := persistAPIRequestEvidence(c); err != nil {
		imageEvidenceFailed.Add(1)
		return
	}
	imageEvidenceWritten.Add(1)
}

func ImageErrorEvidenceHealth() map[string]any {
	return map[string]any{"enabled": evidenceEnabled(), "capacity": cap(imageEvidenceSlots), "active": len(imageEvidenceSlots), "queued": len(imageEvidenceQueue), "accepted": imageEvidenceAccepted.Load(), "written": imageEvidenceWritten.Load(), "skipped": imageEvidenceSkipped.Load(), "failed": imageEvidenceFailed.Load(), "dropped": imageEvidenceDropped.Load()}
}
