package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type apiRequestEvidenceKey struct{}

// APIRequestEvidence observes existing I/O without changing authentication,
// request validation, cancellation, retries or provider semantics.
func APIRequestEvidence() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !evidenceEnabled() || !isBusinessRequest(c.Request.URL.Path) {
			c.Next()
			return
		}
		capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: c.GetString(common.RequestIdKey), CreatedAt: common.GetTimestamp(), Kind: "api_request"})
		if capture == nil {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), apiRequestEvidenceKey{}, capture))
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
		defer func() {
			captureRejectedRequestRemainder(c, capture, request, writer)
			capture.mu.Lock()
			capture.Index.UserID = c.GetInt("id")
			capture.Index.TokenID = c.GetInt("token_id")
			capture.Index.AppID = c.GetInt("token_id")
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
			capture.FinishClient(status, headers)
		}()
		c.Next()
	}
}

func isBusinessRequest(path string) bool {
	for _, prefix := range []string{"/v1", "/v1beta", "/api/v3", "/pg", "/mj", "/kling", "/jimeng"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return strings.Contains(path, "/mj/")
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
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "task_execution", TaskID: task.TaskID, UserID: task.UserId, AppID: task.AppID, TokenID: task.PrivateData.TokenId, ChannelID: task.ChannelId, ClientModel: task.Properties.OriginModelName, CreatedAt: common.GetTimestamp()})
	if capture == nil {
		return ctx, func() {}
	}
	return context.WithValue(ctx, apiRequestEvidenceKey{}, capture), func() { capture.Finish(false) }
}

func persistAPIRequestEvidence(c *ImageErrorEvidence) error {
	config := system_setting.GetTaskRequestEvidenceConfig()
	if c.Index.CreatedAt == 0 {
		c.Index.CreatedAt = common.GetTimestamp()
	}
	c.Index.RetainUntil = evidenceRetentionUntil(config, c.Index.CreatedAt)
	if err := model.CreateTaskRequestEvidence(&c.Index); err != nil {
		return err
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
		if err := persistTaskEvidenceBodyUsing(event, body, TaskRequestEvidenceSourceResponse, c.redactObservedBody); err != nil {
			result = err
		}
	}
	return result
}

func BeginBatchRequestEvidence(ctx context.Context, job *model.BatchJob) (context.Context, func()) {
	if job == nil || apiEvidenceFrom(ctx) != nil {
		return ctx, func() {}
	}
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{Kind: "batch_execution", TaskID: job.Id, UserID: job.UserId, AppID: job.AppID, TokenID: job.TokenId, ChannelID: job.ChannelId, ClientModel: job.PublicModel, CreatedAt: common.GetTimestamp()})
	if capture == nil {
		return ctx, func() {}
	}
	return context.WithValue(ctx, apiRequestEvidenceKey{}, capture), func() { capture.Finish(false) }
}

func HasAPIRequestEvidence(ctx context.Context) bool { return apiEvidenceFrom(ctx) != nil }
