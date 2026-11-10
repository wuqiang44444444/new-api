package service

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/model"
)

func evidenceLegacySHA256(event *model.TaskRequestEvidenceEvent) string {
	// PostgreSQL pads an empty CHAR(64) digest with ASCII spaces.
	return strings.TrimRight(event.Sha256, " ")
}

// ReadEvidenceEventBody loads an entire object for explicit internal inspection.
// HTTP previews and downloads instead use the bounded/streaming read entry.
// Authentication failure cannot distinguish a
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
	digest := evidenceLegacySHA256(event)
	switch {
	case errors.Is(err, ErrTaskRequestEvidenceUnavailable):
		return nil, "missing"
	case errors.Is(err, ErrEvidenceDecryptFailed):
		return nil, "decrypt_failed"
	case errors.Is(err, ErrEvidenceIntegrityFailed):
		return nil, "integrity_failed"
	case err != nil:
		return nil, "read_failed"
	case digest != "" && EvidenceSha256Hex(payload) != digest:
		return nil, "integrity_failed"
	default:
		return payload, "available"
	}
}

// EvidenceEventContent is one explicitly selected preview. Ordinary bodies stay
// raw; WS storage frames become bounded message previews. JSON formatting is a
// separate explicit read operation and never changes the captured bytes.
type EvidenceEventContent struct {
	Meta           *EvidenceObjectMeta
	Body           []byte
	BodyStatus     string
	DisplayLimited bool
	Messages       []EvidenceRealtimeMessage
	FormattedText  *string
	JSONStatus     string
}

// ReadEvidenceEventContent 读取并拆分一条事件对象；历史对象按原始正文返回，
// 元数据尽力从事件行 Detail 解析，解析失败不改变正文的可用性。
func ReadEvidenceEventContent(event *model.TaskRequestEvidenceEvent, expired bool) (*EvidenceEventContent, string) {
	content, status := readEvidenceContentWindow(event, expired)
	if status == "available" && content.Meta != nil && content.Meta.BodyFormat == evidenceRealtimeFormat {
		decodeEvidenceRealtimeWindow(content)
	}
	return content, status
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
