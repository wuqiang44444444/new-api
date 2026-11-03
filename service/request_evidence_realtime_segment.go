package service

import (
	"encoding/base64"
	"encoding/binary"
	"time"

	"github.com/gorilla/websocket"
)

const evidenceRealtimeFormat = "websocket-segment-v1"
const evidenceRealtimeSeconds int64 = 15
const evidenceRealtimeHeader = 32

// Adjacent WS pieces share one direction's budget during asynchronous handoff.
// Queued body blocks retain the global 64 MiB bound. The ordered worker seals
// the predecessor before opening its successor, so physical streams stay bounded.
func (r *imageEvidenceExchange) reservePieceBudget() bool {
	if r.meta.BodyFormat != evidenceRealtimeFormat {
		select {
		case imageEvidenceSlots <- struct{}{}:
			return true
		default:
			return false
		}
	}
	r.pieceBudgetMu.Lock()
	defer r.pieceBudgetMu.Unlock()
	if r.realtimePieces == 0 {
		select {
		case imageEvidenceSlots <- struct{}{}:
		default:
			return false
		}
	}
	r.realtimePieces++
	return true
}

func (r *imageEvidenceExchange) releasePieceBudget() {
	if r.meta.BodyFormat != evidenceRealtimeFormat {
		<-imageEvidenceSlots
		return
	}
	r.pieceBudgetMu.Lock()
	defer r.pieceBudgetMu.Unlock()
	r.realtimePieces--
	if r.realtimePieces == 0 {
		<-imageEvidenceSlots
	}
}

func (c *ImageErrorEvidence) realtimeRecord(stage string) *imageEvidenceExchange {
	c.realtimeMu.Lock()
	defer c.realtimeMu.Unlock()
	if c.realtime == nil {
		c.realtime = make(map[string]*imageEvidenceExchange)
	}
	if record, exists := c.realtime[stage]; exists {
		return record
	}
	record := c.newRecord(stage, EvidenceObjectMeta{BodyFormat: evidenceRealtimeFormat}, false)
	// A failed reservation stays failed; never restart at a message suffix.
	c.realtime[stage] = record
	return record
}

func (r *imageEvidenceExchange) appendRealtimeMessage(messageType int, message []byte, frameErr error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished || r.stopped || !r.openPiece() {
		return
	}
	frameError := errorString(frameErr)
	// Error metadata has the same bounded budget as other captured metadata.
	if len(frameError) > 64<<10 {
		r.stop()
		return
	}
	stored := message
	if messageType == websocket.BinaryMessage {
		stored = nil
	}
	var header [evidenceRealtimeHeader]byte
	binary.BigEndian.PutUint32(header[0:4], uint32(messageType))
	binary.BigEndian.PutUint32(header[4:8], uint32(len(frameError)))
	binary.BigEndian.PutUint64(header[8:16], uint64(time.Now().UnixMilli()))
	binary.BigEndian.PutUint64(header[16:24], uint64(len(message)))
	binary.BigEndian.PutUint64(header[24:32], uint64(len(stored)))
	r.appendBytes(header[:])
	r.appendBytes([]byte(frameError))
	r.appendBytes(stored)
}

// Only selected reads decode the storage framing; payloads are never JSON-decoded here.
type EvidenceRealtimeMessage struct {
	Offset         uint64  `json:"offset"`
	MessageType    int     `json:"message_type"`
	CreatedAtMS    int64   `json:"created_at_ms"`
	OriginalBytes  uint64  `json:"original_bytes"`
	Error          string  `json:"error,omitempty"`
	Text           *string `json:"text"`
	Base64         string  `json:"base64,omitempty"`
	DisplayLimited bool    `json:"display_limited,omitempty"`
	FormattedText  *string `json:"formatted_text,omitempty"`
	JSONStatus     string  `json:"json_status,omitempty"`
}

func decodeEvidenceRealtimeWindow(content *EvidenceEventContent) {
	data := content.Body
	windowBytes := len(data)
	content.Body = nil
	content.Messages = make([]EvidenceRealtimeMessage, 0)
	for len(data) >= evidenceRealtimeHeader {
		errorSize := uint64(binary.BigEndian.Uint32(data[4:8]))
		storedSize := binary.BigEndian.Uint64(data[24:32])
		frame := EvidenceRealtimeMessage{Offset: uint64(windowBytes - len(data)), MessageType: int(int32(binary.BigEndian.Uint32(data[:4]))), CreatedAtMS: int64(binary.BigEndian.Uint64(data[8:16])), OriginalBytes: binary.BigEndian.Uint64(data[16:24])}
		data = data[evidenceRealtimeHeader:]
		// Compare before converting lengths: corrupt input cannot overflow or allocate.
		if errorSize > uint64(len(data)) {
			content.DisplayLimited = true
			return
		}
		frame.Error = string(data[:int(errorSize)])
		data = data[int(errorSize):]
		size := uint64(len(data))
		if storedSize < size {
			size = storedSize
		}
		body := data[:int(size)]
		frame.DisplayLimited = size < storedSize
		if frame.MessageType != websocket.BinaryMessage {
			if EvidenceContentIsText(body) {
				text := string(body)
				frame.Text = &text
			} else {
				frame.Base64 = base64.StdEncoding.EncodeToString(body)
			}
		}
		content.Messages = append(content.Messages, frame)
		data = data[int(size):]
		if frame.DisplayLimited {
			content.DisplayLimited = true
			return
		}
	}
	if len(data) != 0 {
		content.DisplayLimited = true
	}
}
