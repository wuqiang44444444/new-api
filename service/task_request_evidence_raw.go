package service

import (
	"bytes"
	"net/url"

	"github.com/QuantumNous/new-api/common"
)

// 原始证据对象格式（v2）：实际捕获的字节原样保存，仅在管理员
// 逐条查看时解析存储元数据。对象首行是固定标记，第二行是单行元数据 JSON（含原始请求
// 头与查询串），其余字节是未改写的正文。历史对象没有该标记，按原始正文
// 读取；取证不重写正文，也不在保存前解析、脱敏或拒绝任何格式。
const evidenceObjectMagic = "new-api/evidence-v2"

// EvidenceObjectMeta 是对象内嵌的交换元数据。请求头、查询串按捕获原文保存；
// 受保护存储之外不得输出这些内容。
type EvidenceObjectMeta struct {
	BodyFormat  string              `json:"body_format,omitempty"`
	MessageType int                 `json:"message_type,omitempty"`
	RawQuery    string              `json:"raw_query,omitempty"`
	Method      string              `json:"method,omitempty"`
	Target      string              `json:"target,omitempty"`
	StatusCode  int                 `json:"status_code,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	CreatedAt   int64               `json:"created_at,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Query       url.Values          `json:"query,omitempty"`
	Error       string              `json:"error,omitempty"`
}

// parseEvidenceObject 拆分 v2 对象。返回 nil 元数据表示历史对象：
// 整个负载就是原始正文，由调用方按事件行补充展示信息。
func parseEvidenceObject(payload []byte) (*EvidenceObjectMeta, []byte) {
	first := bytes.IndexByte(payload, '\n')
	if first <= 0 || string(payload[:first]) != evidenceObjectMagic {
		return nil, payload
	}
	rest := payload[first+1:]
	second := bytes.IndexByte(rest, '\n')
	if second < 0 {
		second = len(rest)
		rest = nil
	} else {
		rest = rest[second+1:]
	}
	var meta EvidenceObjectMeta
	if err := common.Unmarshal(payload[first+1:first+1+second], &meta); err != nil {
		return nil, payload
	}
	return &meta, rest
}
