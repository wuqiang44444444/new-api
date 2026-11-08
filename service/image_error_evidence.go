package service

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// The carrier holds only bounded active records and identity facts, never a body.
type ImageErrorEvidence struct {
	Index            model.TaskRequestEvidence
	mu               sync.Mutex
	records          map[*imageEvidenceExchange]struct{}
	delivery         *imageEvidenceExchange
	deliveryPrepared bool
	window           *evidenceWindow
	worker           int
	tasks            []model.EvidenceTaskLink
	finished         bool
	realtimeMu       sync.Mutex
	realtime         map[string]*imageEvidenceExchange
}

type imageEvidenceExchange struct {
	mu             sync.Mutex
	capture        *ImageErrorEvidence
	meta           EvidenceObjectMeta
	stage          string
	piece          *evidencePiece
	block          *[evidenceBlockSize]byte
	used           int
	skip           bool
	stopped        bool
	finished       bool
	split          bool
	pieceBudgetMu  sync.Mutex
	realtimePieces int
}

func NewImageErrorEvidence(index model.TaskRequestEvidence) *ImageErrorEvidence {
	return newAPIRequestEvidence(index)
}
func WithImageErrorEvidence(ctx context.Context, capture *ImageErrorEvidence) context.Context {
	if capture == nil {
		return ctx
	}
	return context.WithValue(ctx, apiRequestEvidenceKey{}, capture)
}

func (c *ImageErrorEvidence) newRecord(stage string, meta EvidenceObjectMeta, skip bool) *imageEvidenceExchange {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished || evidenceDiskPressure.Load() || len(c.records) >= cap(imageEvidenceSlots) {
		imageEvidenceDropped.Add(1)
		return nil
	}
	select {
	case evidenceRecordSlots <- struct{}{}:
	default:
		imageEvidenceDropped.Add(1)
		return nil
	}
	if meta.CreatedAt == 0 {
		meta.CreatedAt = common.GetTimestamp()
	}
	if skip {
		meta.Headers = nil
		meta.RawQuery = ""
	} else {
		meta.Headers = cloneEvidenceHeaders(meta.Headers)
	}
	r := &imageEvidenceExchange{capture: c, meta: meta, stage: stage, skip: skip}
	c.records[r] = struct{}{}
	return r
}

func (r *imageEvidenceExchange) openPiece(bodySource *evidencePiece) bool {
	if r == nil || r.stopped {
		return false
	}
	c := r.capture
	now := common.GetTimestamp()
	c.mu.Lock()
	if c.window == nil || now >= c.window.started+evidenceWindowSeconds {
		c.window = &evidenceWindow{started: now}
	}
	window := c.window
	c.mu.Unlock()
	if bodySource != nil && bodySource.window != window {
		return false
	}
	if r.piece != nil && r.piece.window == window && now < r.piece.sealAt {
		if r.piece.aborted.Load() {
			r.stop()
			return false
		}
		return true
	}
	r.flush()
	if r.stopped {
		return false
	}
	if r.piece != nil {
		r.split = true
		r.piece.closed.Store(true)
	}
	if !r.reservePieceBudget() {
		r.stop()
		return false
	}
	meta := r.meta
	meta.CreatedAt = now
	sealAt := window.started + evidenceWindowSeconds
	if meta.BodyFormat == evidenceRealtimeFormat && now+evidenceRealtimeSeconds < sealAt {
		sealAt = now + evidenceRealtimeSeconds
	}
	r.piece = &evidencePiece{capture: c, window: window, meta: meta, stage: r.stage, metadataOnly: r.skip, record: r, sealAt: sealAt, bodySource: bodySource}
	r.piece.pending.Add(1)
	select {
	case evidenceWorkers[c.worker] <- evidencePacket{piece: r.piece}:
		imageEvidenceAccepted.Add(1)
		return true
	default:
		r.piece.pending.Add(-1)
		r.piece.release()
		r.stop()
		return false
	}
}

func (r *imageEvidenceExchange) stop() {
	r.stopped = true
	if r.piece != nil {
		r.piece.aborted.Store(true)
	}
	if r.block != nil {
		releaseEvidenceBlock(r.block)
		r.block = nil
		r.used = 0
	}
}
func (r *imageEvidenceExchange) flush() {
	if r.block == nil {
		return
	}
	r.piece.pending.Add(1)
	select {
	case evidenceWorkers[r.capture.worker] <- evidencePacket{piece: r.piece, block: r.block, n: r.used}:
		r.block = nil
		r.used = 0
	default:
		r.piece.pending.Add(-1)
		r.stop()
	}
}
func (r *imageEvidenceExchange) append(p []byte) {
	if r == nil || r.skip {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished || r.stopped {
		return
	}
	if !r.openPiece(nil) {
		return
	}
	r.appendBytes(p)
}

// Caller holds r.mu and has selected the piece; a message cannot rotate midway.
func (r *imageEvidenceExchange) appendBytes(p []byte) {
	for len(p) > 0 && !r.stopped {
		if r.block == nil {
			select {
			case evidenceBlockSlots <- struct{}{}:
				r.block = evidenceBlocks.Get().(*[evidenceBlockSize]byte)
			default:
				r.stop()
				return
			}
		}
		n := copy(r.block[r.used:], p)
		r.used += n
		p = p[n:]
		if r.used == evidenceBlockSize {
			r.flush()
		}
	}
}
func (r *imageEvidenceExchange) close() {
	r.seal(nil)
}

func (r *imageEvidenceExchange) seal(bodySource *evidencePiece) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return false
	}
	if bodySource != nil && (r.skip || r.piece != nil || !r.openPiece(bodySource)) {
		r.mu.Unlock()
		return false
	}
	if !r.stopped {
		if bodySource == nil && r.meta.BodyFormat != evidenceRealtimeFormat {
			r.openPiece(nil)
		}
		r.flush()
	}
	r.finished = true
	<-evidenceRecordSlots
	if r.piece != nil {
		r.piece.closed.Store(true)
	}
	r.mu.Unlock()
	r.capture.mu.Lock()
	delete(r.capture.records, r)
	r.capture.mu.Unlock()
	return true
}

func (c *ImageErrorEvidence) recordError(stage string, err error) {
	if c == nil || err == nil {
		return
	}
	r := c.newRecord(stage, EvidenceObjectMeta{Error: err.Error()}, false)
	r.close()
}
func RecordImageDeliveryError(ctx context.Context, stage string, err error) {
	capture := apiEvidenceFrom(ctx)
	capture.recordError(stage, err)
}

func ObserveImageHTTPExchange(ctx context.Context, req *http.Request, resp *http.Response, err error, stage string) {
	if req != nil && evidenceInfrastructureRequest(req) {
		return
	}
	capture := apiEvidenceFrom(ctx)
	if capture == nil || resp == nil {
		return
	}
	if _, ok := resp.Body.(*imageEvidenceBody); ok {
		return
	}
	capture.observeResponse(req, resp, err, stage)
}

type imageEvidenceBody struct {
	io.ReadCloser
	capture         *ImageErrorEvidence
	record          *imageEvidenceExchange
	observeIdentity func()
	finish          func()
}

func (b *imageEvidenceBody) Read(p []byte) (int, error) {
	if b.observeIdentity != nil {
		b.observeIdentity()
		b.observeIdentity = nil
	}
	n, err := b.ReadCloser.Read(p)
	b.record.append(p[:n])
	if err != nil && err != io.EOF {
		b.capture.recordError("read_error", err)
	}
	return n, err
}
func (b *imageEvidenceBody) Close() error {
	err := b.ReadCloser.Close()
	b.record.close()
	if b.finish != nil {
		b.finish()
	}
	return err
}
func (c *ImageErrorEvidence) CaptureClientBody(p []byte) {
	c.mu.Lock()
	r := c.delivery
	c.mu.Unlock()
	r.append(p)
}
func (c *ImageErrorEvidence) Finish(_ bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	c.finished = true
	records := make([]*imageEvidenceExchange, 0, len(c.records))
	for r := range c.records {
		records = append(records, r)
	}
	c.mu.Unlock()
	for _, r := range records {
		r.close()
	}
}
func ImageErrorEvidenceHealth() map[string]any {
	return map[string]any{"enabled": evidenceEnabled(), "capacity": cap(imageEvidenceSlots), "active": len(imageEvidenceSlots), "buffer_blocks": len(evidenceBlockSlots), "active_records": len(evidenceRecordSlots), "disk_pressure": evidenceDiskPressure.Load(), "accepted": imageEvidenceAccepted.Load(), "written": imageEvidenceWritten.Load(), "failed": imageEvidenceFailed.Load(), "dropped": imageEvidenceDropped.Load()}
}

// One shared worker tick rotates idle as well as busy long connections.
func (r *imageEvidenceExchange) rotate(piece *evidencePiece, now int64) {
	if now < piece.sealAt {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.piece != piece || r.finished {
		return
	}
	r.flush()
	r.split = true
	piece.closed.Store(true)
	r.piece = nil
}
