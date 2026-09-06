package service

import (
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

// GetEvidenceEventPreviews 为证据详情返回每个事件的正文预览：
// 文本类正文截断预览并对签名 URL 遮盖（管理员视图），二进制只给摘要。
func GetEvidenceEventPreviews(evidenceId int64, events []*model.TaskRequestEvidenceEvent, isRoot bool) map[int64]string {
	previews := make(map[int64]string)
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		return previews
	}
	for _, event := range events {
		if event.ObjectKey == "" {
			continue
		}
		payload, err := store.Get(event.ObjectKey)
		if err != nil || EvidenceSha256Hex(payload) != event.Sha256 {
			continue
		}
		if strings.HasPrefix(event.ContentType, "audio/") || strings.HasPrefix(event.ContentType, "video/") || event.ContentType == "application/octet-stream" {
			continue
		}
		previews[event.Id] = evidencePreviewText(payload, isRoot)
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
