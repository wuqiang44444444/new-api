package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Image error evidence is observation only. A bounded memory reservation covers
// both in-flight captures and queued writes. No DB/file work runs in Read/Write
// or Finish, and failed evidence never changes the business outcome.
var imageEvidenceSlots = make(chan struct{}, 8)
var imageEvidenceQueue = make(chan *ImageErrorEvidence, 8)
var imageEvidenceAccepted, imageEvidenceWritten, imageEvidenceFailed, imageEvidenceDropped, imageEvidenceTruncated atomic.Uint64
var imageEvidenceSkipped atomic.Uint64

type imageEvidenceContextKey struct{}

type ImageErrorEvidence struct {
	Index     model.TaskRequestEvidence
	mu        sync.Mutex
	remaining int64
	records   []*imageEvidenceExchange
	secrets   []string
	failed    bool
	finished  bool
}

type imageEvidenceExchange struct {
	Stage       string
	Method      string
	Target      string
	Status      int
	Headers     http.Header
	ContentType string
	Error       string
	Body        bytes.Buffer
	Observed    int64
	EOF         bool
	Truncated   bool
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
	index.Kind = "image_error"
	return &ImageErrorEvidence{Index: index, remaining: config.MaxResponseBytes}
}

func NewImageTaskErrorEvidence(task *model.Task) *ImageErrorEvidence {
	return NewImageErrorEvidence(model.TaskRequestEvidence{TaskID: task.TaskID, RequestID: task.TaskID, UserID: task.UserId, AppID: task.AppID, TokenID: task.PrivateData.TokenId, ChannelID: task.ChannelId, ClientProtocol: task.ClientProtocol})
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
// parse or normalize it. Successful responses are retained only if this request
// later fails, including SSE error events and failures while saving a result.
func ObserveImageHTTPExchange(ctx context.Context, req *http.Request, resp *http.Response, err error, stage string) {
	capture := ImageErrorEvidenceFrom(ctx)
	if capture == nil {
		return
	}
	if req != nil {
		capture.protectCredentials(req.Header)
		capture.mu.Lock()
		if !capture.finished && req.URL != nil {
			for name, values := range req.URL.Query() {
				if name == "key" || isEvidenceCredentialKey(name) {
					for _, value := range values {
						if value != "" {
							capture.secrets = append(capture.secrets, value)
						}
					}
				}
			}
		}
		capture.mu.Unlock()
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.finished {
		return
	}
	record := &imageEvidenceExchange{Stage: stage, EOF: resp == nil}
	if req != nil {
		record.Method = req.Method
		record.Target = evidenceTargetURL(req)
	}
	if err != nil {
		record.Error = err.Error()
		capture.failed = true
	}
	if resp != nil {
		record.EOF = resp.Body == nil || resp.Body == http.NoBody || (req != nil && req.Method == http.MethodHead)
		record.Status = resp.StatusCode
		record.Headers = resp.Header.Clone()
		record.ContentType = resp.Header.Get("Content-Type")
		if resp.StatusCode >= 400 {
			capture.failed = true
		}
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
	capture.records = append(capture.records, &imageEvidenceExchange{Stage: stage, Error: err.Error(), EOF: true})
	capture.failed = true
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
	} else if err != nil {
		b.record.Error = err.Error()
		b.capture.failed = true
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
			b.capture.failed = true
		}
		b.capture.mu.Unlock()
	}
	return err
}

func (c *ImageErrorEvidence) appendBody(record *imageEvidenceExchange, p []byte) {
	record.Observed += int64(len(p))
	keep := int64(len(p))
	if keep > c.remaining {
		keep = c.remaining
		record.Truncated = true
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
		record = &imageEvidenceExchange{Stage: "client_delivery"}
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
			r.EOF = true
		}
	}
	if !found && status >= 400 {
		c.records = append(c.records, &imageEvidenceExchange{Stage: "client_delivery", Status: status, Headers: headers.Clone(), ContentType: headers.Get("Content-Type"), EOF: true})
	}
	c.mu.Unlock()
	c.Finish(status >= 400)
}
func (c *ImageErrorEvidence) Finish(failed bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		return
	}
	c.finished = true
	c.failed = c.failed || failed
	// There is one queue slot for every memory reservation; never wait on I/O.
	select {
	case imageEvidenceQueue <- c:
		imageEvidenceAccepted.Add(1)
	default:
		imageEvidenceDropped.Add(1)
		<-imageEvidenceSlots
	}
}

func imageEvidenceHasError(r *imageEvidenceExchange) bool {
	if r.Status >= 400 || r.Error != "" {
		return true
	}
	if strings.Contains(r.ContentType, "text/event-stream") {
		for _, line := range bytes.Split(r.Body.Bytes(), []byte("\n")) {
			if bytes.HasPrefix(line, []byte("event: error")) {
				return true
			}
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			var event struct {
				Type  string `json:"type"`
				Error any    `json:"error"`
			}
			if common.Unmarshal(bytes.TrimSpace(line[5:]), &event) == nil && (event.Error != nil || event.Type == "error" || event.Type == "upstream_error" || strings.HasSuffix(event.Type, ".failed")) {
				return true
			}
		}
	}
	if strings.Contains(r.ContentType, "json") {
		var payload struct {
			Error any `json:"error"`
		}
		if common.Unmarshal(r.Body.Bytes(), &payload) == nil && payload.Error != nil {
			return true
		}
	}
	return false
}

func writeImageErrorEvidence(c *ImageErrorEvidence) {
	defer func() {
		if recover() != nil {
			imageEvidenceFailed.Add(1)
		}
		<-imageEvidenceSlots
	}()
	failed := c.failed
	for _, r := range c.records {
		failed = failed || imageEvidenceHasError(r)
	}
	if !failed || len(c.records) == 0 {
		imageEvidenceSkipped.Add(1)
		return
	}
	if err := persistImageErrorEvidence(c); err != nil {
		imageEvidenceFailed.Add(1)
		return
	}
	imageEvidenceWritten.Add(1)
}

// The existing encrypted evidence store, index, Root download and access audit
// are authoritative; this adds no public response body or second evidence DB.
func persistImageErrorEvidence(c *ImageErrorEvidence) error {
	now := common.GetTimestamp()
	c.Index.CreatedAt = now
	c.Index.RetainUntil = evidenceRetentionUntil(system_setting.GetTaskRequestEvidenceConfig(), now)
	if err := model.CreateTaskRequestEvidence(&c.Index); err != nil {
		return err
	}
	var failures []error
	for _, r := range c.records {
		complete := r.EOF && !r.Truncated
		phase := model.TaskRequestEvidencePhaseResponded
		if !complete {
			phase = model.TaskRequestEvidencePhaseTruncated
			imageEvidenceTruncated.Add(1)
		}
		// Keep headers and full error text in the encrypted object, never in the
		// public index or ordinary log. Credential values are removed first.
		body := r.Body.Bytes()
		for _, secret := range c.secrets {
			body = bytes.ReplaceAll(body, []byte(secret), []byte(evidenceRedactedPlaceholder))
		}
		redacted, err := evidenceRedactBody(body, r.ContentType)
		if err != nil {
			failures = append(failures, err)
			redacted = nil
			phase = model.TaskRequestEvidencePhaseUnavailable
		}
		cause := r.Error
		for _, secret := range c.secrets {
			cause = strings.ReplaceAll(cause, secret, evidenceRedactedPlaceholder)
		}
		// A string preserves the response's textual representation after credential
		// redaction. Binary/partial responses remain explicitly marked incomplete.
		headers := EvidenceRedactHeaders(r.Headers)
		for key, value := range headers {
			for _, secret := range c.secrets {
				value = strings.ReplaceAll(value, secret, evidenceRedactedPlaceholder)
			}
			headers[key] = value
		}
		payload, marshalErr := common.Marshal(map[string]any{"headers": headers, "body": string(redacted), "error": cause, "content_type": r.ContentType, "complete": complete && err == nil})
		if marshalErr != nil {
			failures = append(failures, marshalErr)
			continue
		}
		event := &model.TaskRequestEvidenceEvent{EvidenceId: c.Index.Id, Stage: r.Stage, Phase: phase, StatusCode: r.Status, Method: r.Method, Target: r.Target, ContentType: "application/json", ByteCount: r.Observed, Complete: complete && err == nil, Redacted: true}
		if err := persistTaskEvidenceBody(event, payload, TaskRequestEvidenceSourceResponse); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func ImageErrorEvidenceHealth() map[string]any {
	return map[string]any{"enabled": evidenceEnabled(), "capacity": cap(imageEvidenceSlots), "active": len(imageEvidenceSlots), "queued": len(imageEvidenceQueue), "accepted": imageEvidenceAccepted.Load(), "written": imageEvidenceWritten.Load(), "skipped": imageEvidenceSkipped.Load(), "failed": imageEvidenceFailed.Load(), "dropped": imageEvidenceDropped.Load(), "truncated": imageEvidenceTruncated.Load()}
}
