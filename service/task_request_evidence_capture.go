package service

import (
	"bytes"
	"context"
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

// ErrTaskRequestEvidenceUnavailable identifies a failed durable sending check.
var ErrTaskRequestEvidenceUnavailable = errors.New("task sending permission unavailable")

func IsTaskRequestEvidenceUnavailable(err error) bool {
	return errors.Is(err, ErrTaskRequestEvidenceUnavailable)
}

func evidenceEnabled() bool {
	return system_setting.GetTaskRequestEvidenceConfig().Enabled
}

func evidenceMaxBodyBytes() int64 {
	return system_setting.GetTaskRequestEvidenceConfig().MaxBodyBytes
}

// taskRequestEvidenceSession 保存单个客户请求的证据状态。
type taskRequestEvidenceSession struct {
	evidenceID int64
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

// BeginTaskRequestEvidence associates bounded raw captures with the request.
// Evidence is optional observation; configuration, storage and capture failures
// never reject the business request.
func BeginTaskRequestEvidence(c *gin.Context, kind string) error {
	if !evidenceEnabled() || c == nil || c.Request == nil || c.GetInt("id") <= 0 || evidenceSessionFrom(c) != nil {
		return nil
	}
	eligibleKind := mediaEvidenceContextKind(c)
	if eligibleKind == "" || eligibleKind == "task_query" {
		return nil
	}
	if mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path) == "task_submit" {
		kind = eligibleKind
	}
	now := common.GetTimestamp()
	index := &model.TaskRequestEvidence{RequestID: c.GetString(common.RequestIdKey), UserID: c.GetInt("id"), TokenID: c.GetInt("token_id"), AppID: c.GetInt("app_id"), Kind: kind, CreatedAt: now, UpdatedAt: now, RetainUntil: evidenceRetentionUntil(system_setting.GetTaskRequestEvidenceConfig(), now)}
	if index.AppID == 0 {
		index.AppID = index.TokenID
	}
	if err := model.CreateTaskRequestEvidence(index); err != nil {
		evidencePersistWarning("index create", "north_receive", err)
		return nil
	}
	session := &taskRequestEvidenceSession{evidenceID: index.Id}
	c.Set(taskRequestEvidenceContextKey, session)
	if !c.GetBool(taskCreateEvidenceDeliveryKey) {
		c.Writer = &evidenceClientWriter{ResponseWriter: c.Writer}
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		evidencePersistWarning("body cache", "north_receive", err)
		return nil
	}
	reader, err := storage.NewReader()
	if err != nil {
		evidencePersistWarning("body reader", "north_receive", err)
		return nil
	}
	defer reader.Close()
	body, readErr := io.ReadAll(io.LimitReader(reader, max(int64(0), evidenceMaxBodyBytes())))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, Stage: model.TaskRequestEvidenceStageNorthReceive, Method: c.Request.Method, Target: evidenceTargetURL(c.Request), ContentType: c.Request.Header.Get("Content-Type"), CreatedAt: now}
	persistRawEvidenceEvent(event, c.Request.Header, c.Request.URL.Query(), body, errorString(readErr))
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

// EvidenceObjectKey 是事件对象命名规则（<evidenceId>/<seq>.bin）。清理依据
// 该规则定位对象，不依赖 object_key 是否非空，以覆盖“事件已预留但对象完成
// 回写失败”的窗口。
func EvidenceObjectKey(evidenceId int64, seq int64) string {
	return fmt.Sprintf("%d/%d.bin", evidenceId, seq)
}

// CaptureSouthboundTaskRequestEvidence keeps the durable sending check separate
// from optional evidence. The body is captured as the transport consumes it;
// no snapshot, object write or evidence limit can prevent sending.
func CaptureSouthboundTaskRequestEvidence(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	BindAPIRequestEvidence(c, req, info)
	session := evidenceSessionFrom(c)
	if !evidenceEnabled() || session == nil || session.evidenceID <= 0 || req == nil {
		return verifyEvidenceAttemptBeforeSend(c, info)
	}
	closed, err := model.TaskRequestEvidenceBodyClosedForAppend(session.evidenceID, common.GetTimestamp(), EvidenceRetentionSeconds())
	if err != nil {
		evidencePersistWarning("append check", "southbound_send", err)
	}
	if closed || err != nil {
		return verifyEvidenceAttemptBeforeSend(c, info)
	}
	facts := map[string]any{}
	if info != nil && info.ChannelMeta != nil {
		profile := info.ChannelOtherSettings.VideoUpstreamProfile
		if info.ChannelType == constant.ChannelTypeSeedanceLink {
			profile = info.ChannelOtherSettings.VideoUpstreamProtocol.TransportProfile()
		}
		facts["attempt_id"] = int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID))
		facts["client_model"] = info.OriginModelName
		facts["upstream_model"] = info.UpstreamModelName
		facts["channel_id"] = info.ChannelId
		facts["upstream_protocol"] = string(info.ChannelOtherSettings.VideoUpstreamProtocol)
		facts["adapter_version"] = relaycommon.CurrentVideoSouthboundAdapterVersion(info.ChannelType, profile)
		if info.TaskRelayInfo != nil {
			facts["client_protocol"] = info.TaskRelayInfo.ClientProtocol
		}
	}
	model.UpdateTaskRequestEvidenceFacts(session.evidenceID, facts)
	tee := &evidenceResponseBodyTee{session: session, attemptSeq: session.bumpAttemptSeq(), origin: req.Body, buffer: bytes.NewBuffer(nil), maxBytes: evidenceMaxBodyBytes(), stage: model.TaskRequestEvidenceStageSouthboundSend, started: common.GetTimestamp(), method: req.Method, target: evidenceTargetURL(req), contentType: req.Header.Get("Content-Type"), headers: req.Header.Clone()}
	if req.URL != nil {
		tee.requestQuery = req.URL.Query()
	}
	if req.Body == nil || req.Body == http.NoBody {
		tee.origin = http.NoBody
		_ = tee.Close()
	} else {
		req.Body = tee
	}
	*req = *req.WithContext(context.WithValue(req.Context(), taskEvidenceHTTPRequestKey{}, &taskEvidenceHTTPRequest{}))
	// Optional metadata/database work may have waited while recovery took over.
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

// AttachTaskRequestEvidenceUpstreamResponse 包装上游响应体，边转发边捕获。
// 发送已发生，证据写入故障只输出净化诊断，不影响业务响应。
func AttachTaskRequestEvidenceUpstreamResponse(c *gin.Context, resp *http.Response) {
	session := evidenceSessionFrom(c)
	if !evidenceEnabled() || session == nil || session.evidenceID <= 0 || resp == nil {
		return
	}
	if capture := apiEvidenceFrom(c.Request.Context()); capture != nil {
		capture.mu.Lock()
		if record := capture.responseRecords[resp]; record != nil {
			record.taskOwned = true
		}
		capture.mu.Unlock()
	}
	// The response boundary already chose and validated this identity. Match the
	// Task snapshot, including an empty final response after a previous attempt.
	model.SetTaskRequestEvidenceUpstreamRequestID(session.evidenceID, c.GetString(common.UpstreamRequestIdKey))
	maxBytes := system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes
	method := ""
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
	if resp.Request != nil {
		if resp.Request.URL != nil {
			tee.requestQuery = resp.Request.URL.Query()
		}
	}
	if resp.Body == nil {
		tee.origin = http.NoBody
		_ = tee.Close()
		return
	}
	resp.Body = tee
}

// evidenceResponseBodyTee 有界捕获上游响应正文。
type evidenceResponseBodyTee struct {
	requestQuery url.Values
	stage        string
	attemptSeq   int
	readErr      error
	transportErr error
	closed       bool
	closeErr     error
	headers      http.Header
	session      *taskRequestEvidenceSession
	origin       io.ReadCloser
	buffer       *bytes.Buffer
	maxBytes     int64
	started      int64
	mu           sync.Mutex
	statusCode   int
	method       string
	target       string
	contentType  string
}

func (t *evidenceResponseBodyTee) Read(p []byte) (int, error) {
	n, err := t.origin.Read(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	captured := min(int64(n), max(int64(0), t.maxBytes-int64(t.buffer.Len())))
	if !t.closed {
		t.buffer.Write(p[:captured])
	}
	if err != nil && err != io.EOF {
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
	stage := t.stage
	if stage == "" {
		stage = model.TaskRequestEvidenceStageUpstreamResponse
	}
	event := &model.TaskRequestEvidenceEvent{EvidenceId: t.session.evidenceID, Stage: stage, AttemptSeq: t.attemptSeq, StatusCode: t.statusCode, Method: t.method, Target: t.target, ContentType: t.contentType, CreatedAt: t.started}
	persistRawEvidenceEvent(event, t.headers, t.requestQuery, t.buffer.Bytes(), errorString(errors.Join(t.readErr, t.closeErr, t.transportErr)))
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
	event := &model.TaskRequestEvidenceEvent{EvidenceId: session.evidenceID, Stage: model.TaskRequestEvidenceStageTransport, AttemptSeq: int(session.attemptSeq.Load()), CreatedAt: common.GetTimestamp()}
	if err := model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()); err != nil && !errors.Is(err, model.ErrTaskRequestEvidenceBodyClosed) {
		common.SysError("evidence transport event unavailable")
	}
}
