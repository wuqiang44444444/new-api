package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return "transport_error"
}

// EvidenceCountingWriter 包装 http.ResponseWriter，统计实际写出的字节数。
type EvidenceCountingWriter struct {
	delegate http.ResponseWriter
	written  int64
}

func NewEvidenceCountingWriter(delegate http.ResponseWriter) *EvidenceCountingWriter {
	return &EvidenceCountingWriter{delegate: delegate}
}

func (w *EvidenceCountingWriter) Wrap(delegate http.ResponseWriter) http.ResponseWriter {
	w.delegate = delegate
	return w
}

func (w *EvidenceCountingWriter) Written() int64 {
	return w.written
}

func (w *EvidenceCountingWriter) Header() http.Header        { return w.delegate.Header() }
func (w *EvidenceCountingWriter) WriteHeader(statusCode int) { w.delegate.WriteHeader(statusCode) }
func (w *EvidenceCountingWriter) Write(p []byte) (int, error) {
	n, err := w.delegate.Write(p)
	w.written += int64(n)
	return n, err
}

// EvidenceEventPreview separates present readability from historical capture facts.
type EvidenceEventPreview struct {
	Text       string
	BodyStatus string
}

// ReadEvidenceEventBody is shared by preview and download so both report the
// same safe error category. Authentication failure cannot distinguish a wrong
// key from modified ciphertext; never claim that the key alone is the cause.
func ReadEvidenceEventBody(event *model.TaskRequestEvidenceEvent, expired bool) ([]byte, string) {
	if expired {
		return nil, "expired"
	}
	if event.ObjectKey == "" {
		return nil, "not_recorded"
	}
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		return nil, "storage_unavailable"
	}
	payload, err := store.Get(event.ObjectKey)
	switch {
	case errors.Is(err, ErrTaskRequestEvidenceUnavailable):
		return nil, "missing"
	case errors.Is(err, ErrEvidenceDecryptFailed):
		return nil, "decrypt_failed"
	case errors.Is(err, ErrEvidenceIntegrityFailed):
		return nil, "integrity_failed"
	case err != nil:
		return nil, "read_failed"
	case EvidenceSha256Hex(payload) != event.Sha256:
		return nil, "integrity_failed"
	default:
		return payload, "available"
	}
}

// GetEvidenceEventPreviews reads each object once and preserves URL masking.
func GetEvidenceEventPreviews(events []*model.TaskRequestEvidenceEvent, isRoot, expired bool) map[int64]EvidenceEventPreview {
	previews := make(map[int64]EvidenceEventPreview, len(events))
	for _, event := range events {
		payload, status := ReadEvidenceEventBody(event, expired)
		preview := EvidenceEventPreview{BodyStatus: status}
		if status == "available" {
			contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(event.ContentType, ";", 2)[0]))
			if strings.HasPrefix(contentType, "audio/") || strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "image/") || contentType == "application/octet-stream" {
				preview.BodyStatus = "binary"
			} else {
				preview.Text = evidencePreviewText(payload, isRoot)
			}
		}
		previews[event.Id] = preview
	}
	return previews
}

const evidencePreviewLimit = 16 << 10

func evidencePreviewText(payload []byte, isRoot bool) string {
	text := string(payload)
	if !isRoot {
		text = serviceMaskSignedURLs(text)
	}
	if len(text) > evidencePreviewLimit {
		text = text[:evidencePreviewLimit] + "…(truncated)"
	}
	return text
}

var evidenceURLPattern = regexp.MustCompile(`https?://[^\s"<>\x00-\x1f]+`)

func serviceMaskSignedURLs(text string) string {
	return evidenceURLPattern.ReplaceAllStringFunc(text, EvidenceMaskSignedURLs)
}

// EvidenceAdminPreview masks protected URLs before limiting display size.
func EvidenceAdminPreview(text string) string { return evidencePreviewText([]byte(text), false) }
