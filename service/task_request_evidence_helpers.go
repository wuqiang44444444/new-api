package service

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

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

// ReadEvidenceEventBody is shared by content view and download so both report
// the same safe error category. Authentication failure cannot distinguish a
// wrong key from modified ciphertext; never claim that the key alone is the
// cause. 新对象不再写正文摘要，仅在历史事件携带 Sha256 时保留既有校验。
func ReadEvidenceEventBody(event *model.TaskRequestEvidenceEvent, expired bool) ([]byte, string) {
	if expired {
		return nil, "expired"
	}
	if event.ObjectKey == "" && event.Id == 0 {
		return nil, "not_recorded"
	}
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		return nil, "storage_unavailable"
	}
	key := event.ObjectKey
	if key == "" {
		key = EvidenceObjectKey(event.EvidenceId, event.Id)
	}
	payload, err := store.Get(key)
	switch {
	case errors.Is(err, ErrTaskRequestEvidenceUnavailable):
		return nil, "missing"
	case errors.Is(err, ErrEvidenceDecryptFailed):
		return nil, "decrypt_failed"
	case errors.Is(err, ErrEvidenceIntegrityFailed):
		return nil, "integrity_failed"
	case err != nil:
		return nil, "read_failed"
	case event.Sha256 != "" && EvidenceSha256Hex(payload) != event.Sha256:
		return nil, "integrity_failed"
	default:
		return payload, "available"
	}
}

// EvidenceEventContent 是管理员选中一条证据后的按需读取结果：
// 元数据来自对象内嵌信息（历史对象回退事件行 Detail），正文是解密后的
// 原始字节。解析、美化和预览由展示层触发，这里不做任何内容转换。
type EvidenceEventContent struct {
	Meta       *EvidenceObjectMeta
	Body       []byte
	BodyStatus string
}

// ReadEvidenceEventContent 读取并拆分一条事件对象；历史对象按原始正文返回，
// 元数据尽力从事件行 Detail 解析，解析失败不改变正文的可用性。
func ReadEvidenceEventContent(event *model.TaskRequestEvidenceEvent, expired bool) (*EvidenceEventContent, string) {
	payload, status := ReadEvidenceEventBody(event, expired)
	if status != "available" {
		return nil, status
	}
	meta, body := parseEvidenceObject(payload)
	if meta == nil && event.Detail != "" {
		var detail EvidenceObjectMeta
		if common.UnmarshalJsonStr(event.Detail, &detail) == nil {
			meta = &detail
		}
	}
	return &EvidenceEventContent{Meta: meta, Body: body, BodyStatus: "available"}, "available"
}

// EvidenceContentIsText 报告正文是否按 UTF-8 文本返回；二进制内容交给
// 展示层按 base64 处理，不在读取侧猜测格式。
func EvidenceContentIsText(body []byte) bool {
	return utf8.Valid(body)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
