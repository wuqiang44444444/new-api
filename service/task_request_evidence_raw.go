package service

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// 原始证据对象格式（v2）：实际捕获的字节原样保存，解析与美化推迟到管理员
// 逐条查看时进行。对象首行是固定标记，第二行是单行元数据 JSON（含原始请求
// 头与查询串），其余字节是未改写的正文。历史对象没有该标记，按原始正文
// 读取；取证不重写正文，也不在保存前解析、脱敏或拒绝任何格式。
const evidenceObjectMagic = "new-api/evidence-v2"

// EvidenceObjectMeta 是对象内嵌的交换元数据。请求头、查询串按捕获原文保存；
// 受保护存储之外不得输出这些内容。
type EvidenceObjectMeta struct {
	Method      string              `json:"method,omitempty"`
	Target      string              `json:"target,omitempty"`
	StatusCode  int                 `json:"status_code,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	CreatedAt   int64               `json:"created_at,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Query       url.Values          `json:"query,omitempty"`
	Error       string              `json:"error,omitempty"`
}

// buildEvidenceObject 组装 v2 对象：标记行 + 单行元数据 + 原始正文。
// 元数据只含小型定位事实；正文保持捕获时的字节，不做任何转换。
func buildEvidenceObject(meta EvidenceObjectMeta, body []byte) []byte {
	encoded, err := common.Marshal(meta)
	if err != nil {
		encoded = []byte("{}")
	}
	out := make([]byte, 0, len(evidenceObjectMagic)+1+len(encoded)+1+len(body))
	out = append(out, evidenceObjectMagic...)
	out = append(out, '\n')
	out = append(out, encoded...)
	out = append(out, '\n')
	return append(out, body...)
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

// persistRawEvidenceEvent 尽力保存一次交换的事件行与原始对象。任何失败只
// 输出净化诊断，绝不改变业务发送、响应、重试、资金或 Task 状态；证据允许
// 缺失。事件行只承载定位信息（阶段、序号、方法、目标、类型、时间），不再
// 维护阶段状态、完整性、字节数或摘要。
func persistRawEvidenceEvent(event *model.TaskRequestEvidenceEvent, headers http.Header, query url.Values, body []byte, errorNote string) {
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	if err := model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()); err != nil {
		if errors.Is(err, model.ErrTaskRequestEvidenceBodyClosed) {
			return // Retention ends observation, never the business operation.
		}
		evidencePersistWarning("event reserve", event.Stage, err)
		return
	}
	closed, err := model.TaskRequestEvidenceBodyClosedForAppend(event.EvidenceId, common.GetTimestamp(), EvidenceRetentionSeconds())
	if err != nil {
		evidencePersistWarning("append check", event.Stage, err)
		return
	}
	if closed {
		return
	}
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		evidencePersistWarning("object store", event.Stage, errors.New("evidence store not initialized"))
		return
	}
	meta := EvidenceObjectMeta{
		Method:      event.Method,
		Target:      event.Target,
		StatusCode:  event.StatusCode,
		ContentType: event.ContentType,
		CreatedAt:   event.CreatedAt,
		Error:       errorNote,
	}
	if len(headers) > 0 {
		meta.Headers = headers
	}
	if len(query) > 0 {
		meta.Query = query
	}
	if err := store.PutEventBody(event.EvidenceId, event.Id, buildEvidenceObject(meta, body)); err != nil {
		if errors.Is(err, model.ErrTaskRequestEvidenceBodyClosed) {
			return // The publication lock rejected a late object.
		}
		evidencePersistWarning("object write", event.Stage, err)
	}
}

func evidencePersistWarning(step, stage string, err error) {
	common.SysError("evidence save unavailable: step=" + step + " stage=" + stage + " cause=" + common.SanitizeTaskDiagnostic(err.Error()))
}
