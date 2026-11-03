package service

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const evidenceBlockSize = 64 << 10
const evidenceWorkerCount = 4
const evidenceWindowSeconds int64 = 3600

// At most 64 MiB of queued/capturing blocks, plus at most 512 SIO objects.
// Slots are acquired before allocation; neither body length nor connection
// duration can grow the budget. Each object is assigned to one ordered worker.
var evidenceBlockSlots = make(chan struct{}, 1024)
var imageEvidenceSlots = make(chan struct{}, 512)
var evidenceRecordSlots = make(chan struct{}, 512)
var evidenceBlocks = sync.Pool{New: func() any { return new([evidenceBlockSize]byte) }}
var evidenceWorkerSequence atomic.Uint64
var evidenceWorkers [evidenceWorkerCount]chan evidencePacket
var imageEvidenceAccepted, imageEvidenceWritten, imageEvidenceFailed, imageEvidenceDropped atomic.Uint64

type evidencePacket struct {
	piece *evidencePiece
	block *[evidenceBlockSize]byte
	n     int
	fence chan struct{}
}

type evidenceWindow struct {
	started int64
	index   model.TaskRequestEvidence
	facts   model.TaskRequestEvidence
	created bool
	failed  bool
	linked  bool
}

type evidencePiece struct {
	record       *imageEvidenceExchange
	metadataOnly bool
	capture      *ImageErrorEvidence
	window       *evidenceWindow
	meta         EvidenceObjectMeta
	stage        string
	sealAt       int64
	pending      atomic.Int64
	closed       atomic.Bool
	aborted      atomic.Bool
	released     atomic.Bool
	object       *evidenceStreamObject
	event        *model.TaskRequestEvidenceEvent
}

func init() {
	for i := range evidenceWorkers {
		evidenceWorkers[i] = make(chan evidencePacket, 1024)
		go runEvidenceWorker(evidenceWorkers[i])
	}
}

func releaseEvidenceBlock(block *[evidenceBlockSize]byte) {
	evidenceBlocks.Put(block)
	<-evidenceBlockSlots
}

func (p *evidencePiece) release() {
	if !p.released.CompareAndSwap(false, true) {
		return
	}
	if p.object != nil {
		p.object.Abort()
	}
	p.record.releasePieceBudget()
}

func (p *evidencePiece) start() error {
	c := p.capture
	c.mu.Lock()
	facts := c.Index
	tasks := append([]model.EvidenceTaskLink(nil), c.tasks...)
	c.mu.Unlock()
	w := p.window
	if w.failed {
		return errors.New("evidence window unavailable")
	}
	if !w.created {
		w.index = facts
		w.index.Id = 0
		w.index.CreatedAt = w.started
		w.index.RetainUntil = w.index.CreatedAt + EvidenceRetentionSeconds()
		if err := model.CreateTaskRequestEvidence(&w.index); err != nil {
			w.failed = true
			return err
		}
		w.created = true
		w.facts = facts
	}
	p.event = &model.TaskRequestEvidenceEvent{EvidenceId: w.index.Id, Stage: p.stage, Method: p.meta.Method, Target: p.meta.Target, StatusCode: p.meta.StatusCode, ContentType: p.meta.ContentType, CreatedAt: p.meta.CreatedAt}
	if err := model.CreateTaskRequestEvidenceEvent(p.event, EvidenceRetentionSeconds()); err != nil {
		return err
	}
	if !w.linked {
		if err := model.AttachEvidenceTasks(w.index.Id, tasks, EvidenceRetentionSeconds()); err != nil {
			return err
		}
		w.linked = true
	}
	if p.metadataOnly {
		return nil
	}
	store, ok := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	if !ok {
		return errors.New("evidence stream store unavailable")
	}
	object, err := store.beginStream(EvidenceObjectKey(w.index.Id, p.event.Id), w.index.Id)
	if err != nil {
		return err
	}
	p.object = object
	// Only the small storage envelope is serialized. Body bytes pass untouched.
	header, err := common.Marshal(p.meta)
	if err != nil {
		return err
	}
	if len(header) >= 128<<10 {
		return errors.New("evidence envelope exceeds metadata budget")
	}
	_, err = io.WriteString(object, evidenceObjectMagic+"\n")
	if err == nil {
		_, err = object.Write(header)
	}
	if err == nil {
		_, err = object.Write([]byte{'\n'})
	}
	return err
}

func (p *evidencePiece) finish() {
	defer p.release()
	defer func() {
		if recover() != nil {
			imageEvidenceFailed.Add(1)
			common.SysError("request evidence finalization unavailable")
		}
	}()
	if evidenceDiskPressure.Load() {
		p.aborted.Store(true)
	}
	if p.aborted.Load() {
		imageEvidenceDropped.Add(1)
		return
	}
	if p.event == nil {
		if err := p.start(); err != nil {
			imageEvidenceFailed.Add(1)
			return
		}
	}
	c := p.capture
	c.mu.Lock()
	facts := c.Index
	c.mu.Unlock()
	if facts != p.window.facts {
		model.UpdateTaskRequestEvidenceFacts(p.window.index.Id, map[string]any{
			"user_id": facts.UserID, "app_id": facts.AppID, "token_id": facts.TokenID,
			"task_id": facts.TaskID, "attempt_id": facts.AttemptID, "channel_id": facts.ChannelID,
			"client_model": facts.ClientModel, "upstream_model": facts.UpstreamModel,
			"upstream_request_id": facts.UpstreamRequestID, "client_protocol": facts.ClientProtocol,
		})
		p.window.facts = facts
	}
	if p.metadataOnly {
		imageEvidenceWritten.Add(1)
		return
	}
	if err := p.object.Close(); err != nil {
		imageEvidenceFailed.Add(1)
		common.SysError("request evidence stream publication failed")
	} else {
		imageEvidenceWritten.Add(1)
	}
}

func runEvidenceWorker(queue <-chan evidencePacket) {
	active := make(map[*evidencePiece]struct{})
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case packet := <-queue:
			if packet.fence != nil {
				finishEvidencePieces(active)
				close(packet.fence)
				continue
			}
			p := packet.piece
			// Same-record packets are ordered: seal its closed predecessor before
			// opening a successor, keeping one physical stream per direction.
			if p.event == nil {
				finishEvidencePieces(active)
			}
			active[p] = struct{}{}
			p.write(packet.block, packet.n)
			if packet.block != nil {
				releaseEvidenceBlock(packet.block)
			}
			p.pending.Add(-1)
			finishEvidencePieces(active)
		case <-ticker.C:
			for p := range active {
				p.record.rotate(p, common.GetTimestamp())
			}
			finishEvidencePieces(active)
		}
	}
}

func finishEvidencePieces(active map[*evidencePiece]struct{}) {
	for p := range active {
		if p.pending.Load() == 0 && (p.closed.Load() || p.aborted.Load()) {
			p.finish()
			delete(active, p)
		}
	}
}

// FlushRequestEvidence fences submitted work for deterministic verification.
// Relay and process shutdown never wait for this non-core observation queue.
func FlushRequestEvidence(ctx context.Context) error {
	for _, queue := range evidenceWorkers {
		done := make(chan struct{})
		select {
		case queue <- evidencePacket{fence: done}:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Optional observation cannot crash the process, including on store callbacks.
func (p *evidencePiece) write(block *[evidenceBlockSize]byte, n int) {
	defer func() {
		if recover() != nil {
			p.aborted.Store(true)
			imageEvidenceFailed.Add(1)
			common.SysError("request evidence write unavailable")
		}
	}()
	if evidenceDiskPressure.Load() {
		p.aborted.Store(true)
	}
	if p.aborted.Load() {
		return
	}
	if p.event == nil {
		if err := p.start(); err != nil {
			p.aborted.Store(true)
			imageEvidenceFailed.Add(1)
			return
		}
	}
	if !p.metadataOnly && block != nil {
		if _, err := p.object.Write(block[:n]); err != nil {
			p.aborted.Store(true)
			imageEvidenceFailed.Add(1)
		}
	}
}
