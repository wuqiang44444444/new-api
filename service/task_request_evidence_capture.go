package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// 音视频请求证据（一期）采集核心。主体逻辑全部在新增文件内；实际 I/O
// 边界只保留窄调用。证据只描述发生过的事实，永不作为任务状态、资金
// 或重放许可的第二事实源。

const taskRequestEvidenceContextKey = "task_request_evidence_session"

// ErrTaskRequestEvidenceUnavailable 表示证据不可用且请求尚未发送。
// 这是拒绝边界：调用方必须停止履约，返回本地不可重试错误并进入既有
// 释放/退款出口；不得换渠、不得标记上游结果未知。
var ErrTaskRequestEvidenceUnavailable = errors.New("task request evidence storage unavailable")

// ErrTaskRequestEvidenceBodyTooLarge 表示正文超过配置容量上限，
// 在可拒绝的边界给出明确结果，不静默截断合法业务请求。
var ErrTaskRequestEvidenceBodyTooLarge = errors.New("task request evidence body exceeds configured limit")

// IsTaskRequestEvidenceUnavailable 判断错误是否为证据拒绝边界错误。
func IsTaskRequestEvidenceUnavailable(err error) bool {
	return errors.Is(err, ErrTaskRequestEvidenceUnavailable) ||
		errors.Is(err, ErrTaskRequestEvidenceBodyTooLarge)
}

func evidenceEnabled() bool {
	return system_setting.GetTaskRequestEvidenceConfig().Enabled
}

func evidenceMaxBodyBytes() int64 {
	return system_setting.GetTaskRequestEvidenceConfig().MaxBodyBytes
}

// taskRequestEvidenceSession 保存单个客户请求的证据状态。
type taskRequestEvidenceSession struct {
	mu         sync.Mutex
	evidenceID int64
	kind       string
	attemptSeq atomic.Int32
}

func evidenceSessionFrom(c *gin.Context) *taskRequestEvidenceSession {
	if c == nil {
		return nil
	}
	session, _ := c.Get(taskRequestEvidenceContextKey)
	if session == nil {
		return nil
	}
	typed, _ := session.(*taskRequestEvidenceSession)
	return typed
}

// BeginTaskRequestEvidence 在认证与请求体缓存后建立证据会话：
// 同步持久化北向接收证据（方法、路径、脱敏头、脱敏查询与完整正文）。
// 写入不可用或正文超限时返回拒绝边界错误——此时尚未建立资金 hold，
// 请求以本地错误结束，不产生任何 Provider 调用。
func BeginTaskRequestEvidence(c *gin.Context, kind string) error {
	if !evidenceEnabled() || c == nil || c.Request == nil || c.GetInt("id") <= 0 {
		return nil
	}
	if evidenceSessionFrom(c) != nil {
		return nil
	}
	if err := system_setting.ValidateTaskRequestEvidenceConfig(system_setting.GetTaskRequestEvidenceConfig()); err != nil {
		return fmt.Errorf("%w: invalid configuration", ErrTaskRequestEvidenceUnavailable)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return fmt.Errorf("evidence read request body failed: %w", err)
	}
	if storage.Size() > evidenceMaxBodyBytes() {
		return fmt.Errorf("%w: north body %d bytes exceeds limit %d",
			ErrTaskRequestEvidenceBodyTooLarge, storage.Size(), evidenceMaxBodyBytes())
	}
	body, err := storage.Bytes()
	if err != nil {
		return fmt.Errorf("evidence read request body failed: %w", err)
	}

	session := &taskRequestEvidenceSession{kind: kind}
	c.Set(taskRequestEvidenceContextKey, session)
	if err := evidencePersistNorth(c, session, body); err != nil {
		return err
	}
	c.Writer = &evidenceClientWriter{ResponseWriter: c.Writer}
	return nil
}

// evidencePersistNorth 先建立索引及不可用事件，再提交加密正文及完成标记。
func evidencePersistNorth(c *gin.Context, session *taskRequestEvidenceSession, body []byte) error {
	now := common.GetTimestamp()
	config := system_setting.GetTaskRequestEvidenceConfig()
	evidence := &model.TaskRequestEvidence{
		RequestID:   c.GetString(common.RequestIdKey),
		UserID:      c.GetInt("id"),
		TokenID:     c.GetInt("token_id"),
		AppID:       c.GetInt("app_id"),
		Kind:        session.kind,
		CreatedAt:   now,
		UpdatedAt:   now,
		RetainUntil: evidenceRetentionUntil(config, now),
	}
	if c.Request.URL != nil {
		evidence.UpstreamProtocol = c.Request.URL.Path
	}
	if err := model.CreateTaskRequestEvidence(evidence); err != nil {
		return fmt.Errorf("%w: create evidence index failed: %v", ErrTaskRequestEvidenceUnavailable, err)
	}
	session.evidenceID = evidence.Id

	headers := EvidenceRedactHeaders(c.Request.Header)
	query := url.Values{}
	if c.Request.URL != nil {
		query = c.Request.URL.Query()
	}
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		return fmt.Errorf("%w: store not initialized", ErrTaskRequestEvidenceUnavailable)
	}
	event := &model.TaskRequestEvidenceEvent{
		EvidenceId:  evidence.Id,
		Seq:         1,
		Stage:       model.TaskRequestEvidenceStageNorthReceive,
		Phase:       model.TaskRequestEvidencePhaseCompleted,
		Method:      c.Request.Method,
		Target:      c.Request.URL.Path,
		ContentType: c.Request.Header.Get("Content-Type"),
		ByteCount:   int64(len(body)),
		Complete:    true,
		Redacted:    true,
		Detail: evidenceMarshalDetail(map[string]any{
			"headers": headers,
			"query":   EvidenceRedactQueryParams(query),
		}),
		CreatedAt: now,
	}
	if err := persistTaskEvidenceBody(event, body); err != nil {
		return err
	}
	return nil
}

func (s *taskRequestEvidenceSession) bumpAttemptSeq() int {
	return int(s.attemptSeq.Add(1))
}

func evidenceRetentionUntil(config system_setting.TaskRequestEvidenceConfig, now int64) int64 {
	if config.RetentionDays <= 0 {
		return 0
	}
	return now + int64(config.RetentionDays)*86400
}

func evidenceObjectKey(evidenceId int64, seq int64) string {
	return fmt.Sprintf("%d/%d.bin", evidenceId, seq)
}

func evidenceMarshalDetail(payload map[string]any) string {
	encoded, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// CaptureSouthboundTaskRequestEvidence 在请求字节发送前持久化最终南向证据。
// 位于 doRequest 内 MarkTaskCreateAttemptUpstreamStarted 之前：
// 证据写入失败时返回拒绝边界错误，调用方停止履约且不标记 unknown。
func CaptureSouthboundTaskRequestEvidence(
	c *gin.Context,
	req *http.Request,
	info *relaycommon.RelayInfo,
) (captureErr error) {
	session := evidenceSessionFrom(c)
	if !evidenceEnabled() || session == nil || req == nil {
		return nil
	}
	// A failed write may also have outlived the recovery window. Preserve a
	// recovered/unknown hold rather than releasing it using stale local state.
	defer func() {
		if captureErr != nil {
			if err := verifyEvidenceAttemptBeforeSend(c, info); err != nil {
				captureErr = err
			}
		}
	}()
	body, err := evidenceSnapshotRequestBody(req, evidenceMaxBodyBytes())
	if err != nil {
		return err
	}
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		return fmt.Errorf("%w: store not initialized", ErrTaskRequestEvidenceUnavailable)
	}
	session.mu.Lock()
	evidenceID := session.evidenceID
	attemptSeq := session.bumpAttemptSeq()
	session.mu.Unlock()
	if evidenceID <= 0 {
		return fmt.Errorf("%w: evidence index missing", ErrTaskRequestEvidenceUnavailable)
	}

	profile := info.ChannelOtherSettings.VideoUpstreamProfile
	if info.ChannelType == constant.ChannelTypeSeedanceLink {
		profile = info.ChannelOtherSettings.VideoUpstreamProtocol.TransportProfile()
	}
	adapterVersion := relaycommon.CurrentVideoSouthboundAdapterVersion(info.ChannelType, profile)
	facts := map[string]any{
		"attempt_id":        int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID)),
		"client_model":      info.OriginModelName,
		"upstream_model":    info.UpstreamModelName,
		"channel_id":        info.ChannelId,
		"upstream_protocol": string(info.ChannelOtherSettings.VideoUpstreamProtocol),
		"adapter_version":   adapterVersion,
	}
	// ClientProtocol 位于嵌入的 *TaskRelayInfo；音频等无任务路径为 nil，
	// 必须守卫，否则运行时空指针崩溃。
	if info.TaskRelayInfo != nil {
		facts["client_protocol"] = info.TaskRelayInfo.ClientProtocol
	}
	model.UpdateTaskRequestEvidenceFacts(evidenceID, facts)

	event := &model.TaskRequestEvidenceEvent{
		EvidenceId:  evidenceID,
		Seq:         0,
		Stage:       model.TaskRequestEvidenceStageSouthboundSend,
		AttemptSeq:  attemptSeq,
		Phase:       model.TaskRequestEvidencePhasePrepared,
		Method:      req.Method,
		Target:      evidenceTargetURL(req),
		ContentType: req.Header.Get("Content-Type"),
		ByteCount:   int64(len(body)),
		Complete:    true,
		Redacted:    true,
		Detail: evidenceMarshalDetail(map[string]any{
			"headers": EvidenceRedactHeaders(req.Header),
			"host":    req.URL.Host,
		}),
	}
	if err := persistTaskEvidenceBody(event, body); err != nil {
		return err
	}
	return verifyEvidenceAttemptBeforeSend(c, info)
}

// evidenceTargetURL 记录脱敏后的目标地址：去掉查询串与用户信息，
// 只保留 scheme+host+path 的业务目标语义。
func evidenceTargetURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	target := *req.URL
	target.RawQuery = ""
	target.User = nil
	return target.String()
}

// evidenceSnapshotRequestBody 在不动重放契约的前提下快照南向请求正文：
// 优先 req.GetBody（多数任务/音频正文是字节缓冲），否则读出并原位恢复。
// 超限返回拒绝边界错误。
func evidenceSnapshotRequestBody(req *http.Request, maxBytes int64) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		snapshot, err := req.GetBody()
		if err == nil {
			defer snapshot.Close()
			return evidenceReadBounded(snapshot, maxBytes)
		}
	}
	limited := io.LimitReader(req.Body, maxBytes+1)
	snapshot, readErr := io.ReadAll(limited)
	if readErr != nil {
		return nil, fmt.Errorf("evidence snapshot southbound body failed: %w", readErr)
	}
	if int64(len(snapshot)) > maxBytes {
		return nil, fmt.Errorf("%w: southbound body exceeds limit %d", ErrTaskRequestEvidenceBodyTooLarge, maxBytes)
	}
	req.Body = io.NopCloser(bytes.NewReader(snapshot))
	if req.ContentLength == -1 {
		req.ContentLength = int64(len(snapshot))
	}
	return snapshot, nil
}

func evidenceReadBounded(reader io.Reader, maxBytes int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("evidence snapshot read failed: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("%w: body exceeds limit %d", ErrTaskRequestEvidenceBodyTooLarge, maxBytes)
	}
	return payload, nil
}

// AttachTaskRequestEvidenceUpstreamResponse 包装上游响应体，边转发边捕获。
// 发送已发生，证据写入故障只告警并标记不完整，绝不影响业务响应。
func AttachTaskRequestEvidenceUpstreamResponse(c *gin.Context, resp *http.Response) {
	session := evidenceSessionFrom(c)
	if !evidenceEnabled() || session == nil || resp == nil || resp.Body == nil {
		return
	}
	for _, key := range []string{common.RequestIdKey, "X-Request-Id", "Request-Id", "X-Tt-Logid"} {
		if id := resp.Header.Get(key); id != "" {
			model.UpsertTaskRequestEvidenceUpstreamRequestID(session.evidenceID, id)
			break
		}
	}
	maxBytes := system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes
	method := "GET"
	if resp.Request != nil && resp.Request.Method != "" {
		method = resp.Request.Method
	}
	tee := &evidenceResponseBodyTee{
		session:     session,
		attemptSeq:  int(session.attemptSeq.Load()),
		origin:      resp.Body,
		buffer:      bytes.NewBuffer(nil),
		maxBytes:    maxBytes,
		started:     common.GetTimestamp(),
		statusCode:  resp.StatusCode,
		method:      method,
		target:      evidenceTargetURL(resp.Request),
		contentType: resp.Header.Get("Content-Type"),
		headers:     resp.Header.Clone(),
	}
	resp.Body = tee
}

// evidenceResponseBodyTee 有界捕获上游响应正文。
type evidenceResponseBodyTee struct {
	stage       string
	attemptSeq  int
	eof         bool
	readErr     error
	closed      bool
	closeErr    error
	headers     http.Header
	session     *taskRequestEvidenceSession
	origin      io.ReadCloser
	buffer      *bytes.Buffer
	maxBytes    int64
	started     int64
	observed    int64
	mu          sync.Mutex
	phase       string
	statusCode  int
	method      string
	target      string
	contentType string
}

func (t *evidenceResponseBodyTee) Read(p []byte) (int, error) {
	n, err := t.origin.Read(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observed += int64(n)
	remaining := max(int64(0), t.maxBytes-int64(t.buffer.Len()))
	captured := min(int64(n), remaining)
	t.buffer.Write(p[:int(captured)])
	if captured < int64(n) {
		t.phase = model.TaskRequestEvidencePhaseTruncated
	}
	if err == io.EOF {
		t.eof = true
	} else if err != nil {
		t.readErr = err
	}
	return n, err
}

func (t *evidenceResponseBodyTee) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return t.closeErr
	}
	t.closed = true
	t.closeErr = t.origin.Close()
	if t.session.evidenceID <= 0 {
		return t.closeErr
	}
	phase := model.TaskRequestEvidencePhaseCompleted
	complete := t.eof && t.readErr == nil && t.phase == "" && t.closeErr == nil
	if !complete {
		phase = model.TaskRequestEvidencePhaseFailed
	}
	if t.phase != "" {
		phase = t.phase
	}
	stage := t.stage
	if stage == "" {
		stage = model.TaskRequestEvidenceStageUpstreamResponse
	}
	event := &model.TaskRequestEvidenceEvent{
		EvidenceId: t.session.evidenceID, Stage: stage,
		AttemptSeq: t.attemptSeq, Phase: phase, StatusCode: t.statusCode, Method: t.method,
		Target: t.target, ContentType: t.contentType, ByteCount: t.observed, Complete: complete,
		Detail: evidenceMarshalDetail(map[string]any{"eof": t.eof, "truncated": t.phase != "", "read_failed": t.readErr != nil, "close_failed": t.closeErr != nil, "headers": EvidenceRedactHeaders(t.headers)}),
	}
	if err := persistTaskEvidenceBody(event, t.buffer.Bytes()); err != nil {
		common.SysError("evidence response unavailable")
	}
	t.buffer.Reset()
	return t.closeErr
}

// A transport failure has no response body, but must still be visible in the
// request timeline. Do not persist raw network errors containing credentials.
func CaptureTaskEvidenceTransportFailure(c *gin.Context) {
	session := evidenceSessionFrom(c)
	if session == nil || session.evidenceID <= 0 {
		return
	}
	event := &model.TaskRequestEvidenceEvent{EvidenceId: session.evidenceID, Stage: model.TaskRequestEvidenceStageTransport, AttemptSeq: int(session.attemptSeq.Load()), Phase: model.TaskRequestEvidencePhaseFailed, CreatedAt: common.GetTimestamp(), Detail: evidenceMarshalDetail(map[string]any{"response_received": false})}
	if err := model.CreateTaskRequestEvidenceEvent(event); err != nil {
		common.SysError("evidence transport event unavailable")
	}
}
