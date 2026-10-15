package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type apiRequestEvidenceKey struct{}

type apiRequestEvidenceFinishKey struct{}

type taskEvidenceHTTPRequestKey struct{}

// A declaration owns only the first actual HTTP exchange. Redirects keep the
// context but must still be observed as separate, previously unowned exchanges.
type taskEvidenceHTTPRequest struct{ claimed atomic.Bool }

// APIRequestEvidence observes registered media operations without changing
// authentication, routing or provider semantics. Task-owned exchanges share
// their durable index; unowned exchanges remain on that same timeline.
func APIRequestEvidence() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Dynamic plugin routes run in an inner Gin engine. Finish the existing
		// exchange with its authenticated context instead of starting a second
		// observer or losing its task session and query parameters.
		if finish, ok := c.Request.Context().Value(apiRequestEvidenceFinishKey{}).(func(*gin.Context)); ok {
			if c.FullPath() != "" {
				defer finish(c)
			}
			c.Next()
			return
		}
		if !evidenceEnabled() || mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path) == "" {
			c.Next()
			return
		}
		capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: c.GetString(common.RequestIdKey), CreatedAt: common.GetTimestamp(), Kind: mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path)})
		if capture == nil {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), apiRequestEvidenceKey{}, capture))
		capture.deliveryStatsOnly = strings.HasSuffix(c.Request.URL.Path, "/content") || strings.Contains(c.Request.URL.Path, "/mj/image/")
		if capture.deliveryStatsOnly {
			capture.remaining = 0
		}
		capture.suppressSourceURLs = strings.HasPrefix(c.Request.URL.Path, "/v1/assets") || strings.HasPrefix(c.Request.URL.Path, "/v1/asset-groups")
		capture.protectCredentials(c.Request.Header)
		request := capture.requestRecord(c.Request, model.TaskRequestEvidenceStageNorthReceive)
		if c.Request.Body != nil {
			c.Request.Body = &imageEvidenceBody{ReadCloser: c.Request.Body, capture: capture, record: request}
		}
		original := c.Writer
		control, _ := c.Request.Context().Value(apiEvidenceHTTPControlKey{}).(*http.ResponseController)
		writer := &apiEvidenceWriter{imageEvidenceWriter: imageEvidenceWriter{ResponseWriter: original, capture: capture}, control: control}
		c.Writer = writer
		finish := func(c *gin.Context) {
			capture.mu.Lock()
			finished := capture.finished
			capture.mu.Unlock()
			if finished {
				return
			}
			if capture.Index.Kind == "task_submit" {
				kind := mediaEvidenceContextKind(c)
				if kind == "" {
					capture.Abandon()
					return
				}
				capture.Index.Kind = kind
			}
			if session := evidenceSessionFrom(c); session != nil && session.evidenceID > 0 {
				// 北向及交付由有效任务索引持有；南向响应按实际对象归属去重，
				// 未接管的 HTTP、握手和帧继续保留，不创建第二个索引。
				FinishTaskRequestEvidenceClientDelivery(c)
				capture.mu.Lock()
				unowned := capture.records[:0]
				for _, record := range capture.records {
					if record.Stage != model.TaskRequestEvidenceStageNorthReceive && record.Stage != model.TaskRequestEvidenceStageClientDelivery && !record.taskOwned {
						unowned = append(unowned, record)
					}
				}
				capture.records = unowned
				capture.Index.Id = session.evidenceID
				capture.mu.Unlock()
				if len(unowned) == 0 {
					capture.Abandon()
				} else {
					capture.Finish(false)
				}
				return
			}
			if c.GetInt("id") > 0 {
				taskID := c.Param("task_id")
				if taskID == "" {
					taskID = c.Param("video_id")
				}
				if taskID == "" && capture.Index.Kind == "task_query" {
					taskID = c.Param("key")
				}
				if taskID != "" {
					appID := c.GetInt("app_id")
					if appID <= 0 {
						appID = c.GetInt("token_id")
					}
					task, exists, err := model.GetByTaskIDForApp(c.GetInt("id"), appID, taskID)
					if err != nil {
						capture.Abandon()
						return
					}
					if exists {
						index, err := taskMediaEvidenceIndex(task)
						if err != nil || index == nil {
							capture.Abandon()
							return
						}
						capture.Index = *index
					}
				}
			}
			if capture.Index.Kind == "task_query" && !strings.HasPrefix(c.Request.URL.Path, "/suno/") {
				capture.Abandon()
				return
			}
			captureRejectedRequestRemainder(c, capture, request, writer)
			capture.mu.Lock()
			capture.Index.UserID = c.GetInt("id")
			capture.Index.TokenID = c.GetInt("token_id")
			capture.Index.AppID = c.GetInt("token_id")
			if c.GetInt("app_id") > 0 {
				capture.Index.AppID = c.GetInt("app_id")
			}
			if capture.Index.ChannelID == 0 {
				capture.Index.ChannelID = c.GetInt("channel_id")
			}
			if capture.Index.ClientModel == "" {
				capture.Index.ClientModel = c.GetString("original_model")
			}
			capture.mu.Unlock()
			status, headers := original.Status(), original.Header()
			if writer.hijacked {
				status, headers = writer.handshakeStatus, writer.handshakeHeaders
			}
			if finishMidjourneyRequestEvidence(c, capture, request, status, headers) || finishSunoBatchRequestEvidence(c, capture, request, status, headers) {
				return
			}
			capture.FinishClient(status, headers)
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), apiRequestEvidenceFinishKey{}, finish))
		defer finish(c)
		c.Next()
	}
}

func newAPIRequestEvidence(index model.TaskRequestEvidence) *ImageErrorEvidence {
	if !evidenceEnabled() {
		return nil
	}
	return &ImageErrorEvidence{Index: index, allRequests: true, remaining: system_setting.GetTaskRequestEvidenceConfig().MaxResponseBytes}
}

func apiEvidenceFrom(ctx context.Context) *ImageErrorEvidence {
	if ctx == nil {
		return nil
	}
	capture, _ := ctx.Value(apiRequestEvidenceKey{}).(*ImageErrorEvidence)
	return capture
}

// BindAPIRequestEvidence propagates only the observation carrier; the outbound
// request keeps its original timeout and cancellation behavior.
func BindAPIRequestEvidence(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) {
	if c == nil || c.Request == nil || req == nil {
		return
	}
	capture := apiEvidenceFrom(c.Request.Context())
	if capture == nil {
		return
	}
	*req = *req.WithContext(context.WithValue(req.Context(), apiRequestEvidenceKey{}, capture))
	if info != nil {
		capture.mu.Lock()
		capture.Index.ChannelID = info.ChannelId
		capture.Index.ClientModel = info.OriginModelName
		capture.Index.UpstreamModel = info.UpstreamModelName
		capture.mu.Unlock()
	}
}

func (c *ImageErrorEvidence) requestRecord(req *http.Request, stage string) *imageEvidenceExchange {
	c.protectCredentials(req.Header)
	c.mu.Lock()
	defer c.mu.Unlock()
	record := &imageEvidenceExchange{Stage: stage, Method: req.Method, Target: evidenceTargetURL(req), Headers: req.Header.Clone(), ContentType: req.Header.Get("Content-Type"), EOF: req.Body == nil || req.Body == http.NoBody, CreatedAt: common.GetTimestamp()}
	if req.URL != nil {
		record.Query = EvidenceRedactQueryParams(req.URL.Query())
		for key, values := range req.URL.Query() {
			if isEvidenceCredentialKey(key) || isEvidenceSignedQueryKey(key) {
				for _, value := range values {
					if value != "" {
						c.secrets = append(c.secrets, value)
					}
				}
			}
		}
	}
	c.records = append(c.records, record)
	return record
}

type apiEvidenceTransport struct{ http.RoundTripper }

func WithAPIRequestEvidenceTransport(transport http.RoundTripper) http.RoundTripper {
	if _, ok := transport.(*apiEvidenceTransport); ok {
		return transport
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &apiEvidenceTransport{transport}
}

func (t *apiEvidenceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	capture := apiEvidenceFrom(req.Context())
	if capture == nil {
		return t.RoundTripper.RoundTrip(req)
	}
	request := capture.requestRecord(req, model.TaskRequestEvidenceStageSouthboundSend)
	declaration, _ := req.Context().Value(taskEvidenceHTTPRequestKey{}).(*taskEvidenceHTTPRequest)
	if declaration != nil && declaration.claimed.CompareAndSwap(false, true) {
		capture.mu.Lock()
		request.taskOwned = true
		capture.mu.Unlock()
	}
	if req.Body != nil {
		req.Body = &imageEvidenceBody{ReadCloser: req.Body, capture: capture, record: request}
	}
	resp, err := t.RoundTripper.RoundTrip(req)
	capture.mu.Lock()
	if req.ContentLength >= 0 && request.Observed == req.ContentLength {
		request.EOF = true
	}
	capture.mu.Unlock()
	capture.mu.Lock()
	response := &imageEvidenceExchange{Stage: model.TaskRequestEvidenceStageUpstreamResponse, Method: req.Method, Target: evidenceTargetURL(req), CreatedAt: common.GetTimestamp(), EOF: resp == nil}
	if err != nil {
		response.Error = "transport_error"
	}
	if resp != nil {
		if capture.responseRecords == nil {
			capture.responseRecords = make(map[*http.Response]*imageEvidenceExchange)
		}
		capture.responseRecords[resp] = response
		response.Status = resp.StatusCode
		response.Headers = resp.Header.Clone()
		response.ContentType = resp.Header.Get("Content-Type")
		response.EOF = resp.Body == nil || resp.Body == http.NoBody || req.Method == http.MethodHead
		if resp.Body != nil {
			resp.Body = &imageEvidenceBody{ReadCloser: resp.Body, capture: capture, record: response}
		}
	}
	capture.records = append(capture.records, response)
	capture.mu.Unlock()
	return resp, err
}

func (t *apiEvidenceTransport) CloseIdleConnections() {
	if closer, ok := t.RoundTripper.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// A background execution keeps the durable Task identity and existing timeout.
func BeginBackgroundRequestEvidence(ctx context.Context, task *model.Task) (context.Context, func()) {
	if task == nil || apiEvidenceFrom(ctx) != nil {
		return ctx, func() {}
	}
	index, err := taskMediaEvidenceIndex(task)
	if err != nil {
		common.SysError("evidence background index unavailable")
		return ctx, func() {}
	}
	if index == nil {
		return ctx, func() {}
	}
	capture := newAPIRequestEvidence(*index)
	if capture == nil {
		return ctx, func() {}
	}
	return context.WithValue(ctx, apiRequestEvidenceKey{}, capture), func() { capture.Finish(false) }
}

// BeginBatchRequestEvidence preserves the existing execution hook without
// observing Batch/file operations, which are outside media evidence scope.
func BeginBatchRequestEvidence(ctx context.Context, job *model.BatchJob) (context.Context, func()) {
	return ctx, func() {}
}

func persistAPIRequestEvidence(c *ImageErrorEvidence) error {
	config := system_setting.GetTaskRequestEvidenceConfig()
	if c.Index.CreatedAt == 0 {
		c.Index.CreatedAt = common.GetTimestamp()
	}
	if c.Index.Id == 0 {
		c.Index.RetainUntil = evidenceRetentionUntil(config, c.Index.CreatedAt)
		if err := model.CreateTaskRequestEvidence(&c.Index); err != nil {
			return err
		}
	}
	var result error
	for seq, r := range c.records {
		phase := model.TaskRequestEvidencePhaseCompleted
		complete := r.EOF && !r.Truncated && r.Error == ""
		if r.Truncated {
			phase = model.TaskRequestEvidencePhaseTruncated
		} else if !complete {
			phase = model.TaskRequestEvidencePhaseFailed
		}
		body := append([]byte(nil), r.Body.Bytes()...)
		headers := EvidenceRedactHeaders(r.Headers)
		for key, value := range headers {
			for _, secret := range c.secrets {
				if value == secret {
					headers[key] = evidenceRedactedPlaceholder
					break
				}
				if len(secret) >= 8 {
					headers[key] = strings.ReplaceAll(headers[key], secret, evidenceRedactedPlaceholder)
				}
			}
		}

		formatInvalid := false
		if _, err := evidenceRedactBody(body, r.ContentType); err != nil {
			formatInvalid = true
		}
		event := &model.TaskRequestEvidenceEvent{EvidenceId: c.Index.Id, AttemptSeq: seq + 1, Stage: r.Stage, Phase: phase, StatusCode: r.Status, Method: r.Method, Target: r.Target, ContentType: r.ContentType, ByteCount: r.Observed, Complete: complete, CreatedAt: r.CreatedAt, Detail: evidenceMarshalDetail(map[string]any{"headers": headers, "query": r.Query, "body_format_invalid": formatInvalid, "body_not_read": !r.EOF && r.Observed == 0, "eof": r.EOF, "truncated": r.Truncated, "read_failed": r.Error != "", "io_failed": r.Error != ""})}
		if c.deliveryStatsOnly && (r.Stage == model.TaskRequestEvidenceStageClientDelivery || r.Stage == model.TaskRequestEvidenceStageUpstreamResponse) {
			event.Complete = r.EOF && r.Error == ""
			event.Phase = model.TaskRequestEvidencePhaseCompleted
			if !event.Complete {
				event.Phase = model.TaskRequestEvidencePhaseFailed
			}
			event.Detail = evidenceMarshalDetail(map[string]any{"media_body_archived": false, "io_failed": r.Error != ""})
			if err := model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()); err != nil && !errors.Is(err, model.ErrTaskRequestEvidenceBodyClosed) {
				result = err
			}
			continue
		}
		if err := persistTaskEvidenceBodyUsing(event, body, TaskRequestEvidenceSourceResponse, c.redactObservedBody); err != nil {
			result = err
		}
	}
	return result
}

func HasAPIRequestEvidence(ctx context.Context) bool { return apiEvidenceFrom(ctx) != nil }
