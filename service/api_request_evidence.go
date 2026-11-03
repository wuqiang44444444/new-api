package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type apiRequestEvidenceKey struct{}
type apiRequestEvidenceFinishKey struct{}

func APIRequestEvidence() gin.HandlerFunc {
	return func(c *gin.Context) {
		if finish, ok := c.Request.Context().Value(apiRequestEvidenceFinishKey{}).(func(*gin.Context)); ok {
			if c.FullPath() != "" {
				defer finish(c)
			}
			c.Next()
			return
		}
		kind := mediaEvidenceRequestKind(c.Request.Method, c.Request.URL.Path)
		// The inner plugin engine has already identified a registered business
		// route. Its paths are declared by plugins, not limited to vendor prefixes.
		if kind == "" && c.FullPath() != "" && c.GetString("route_tag") == "relay" {
			kind = "api_request"
		}
		if !evidenceEnabled() || evidenceDiskPressure.Load() || kind == "" {
			c.Next()
			return
		}
		capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: c.GetString(common.RequestIdKey), Kind: kind, CreatedAt: common.GetTimestamp()})
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), apiRequestEvidenceKey{}, capture))
		observeIdentity := func() { updateRequestEvidenceIdentity(c, capture) }
		request := capture.requestRecord(c.Request, model.TaskRequestEvidenceStageNorthReceive)
		if c.Request.Body != nil && c.Request.Body != http.NoBody {
			c.Request.Body = &imageEvidenceBody{ReadCloser: c.Request.Body, capture: capture, record: request, observeIdentity: observeIdentity}
		} else {
			// A bodyless handshake must not hold a record for the WS lifetime.
			request.close()
		}
		original := c.Writer
		writer := &apiEvidenceWriter{imageEvidenceWriter: imageEvidenceWriter{ResponseWriter: original, capture: capture, observeIdentity: observeIdentity, requestPath: c.Request.URL.Path}}
		c.Writer = writer
		finish := func(authenticated *gin.Context) {
			updateRequestEvidenceIdentity(authenticated, capture)
			writer.prepareDelivery()
			capture.Finish(false)
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), apiRequestEvidenceFinishKey{}, finish))
		defer finish(c)
		c.Next()
	}
}

func updateRequestEvidenceIdentity(c *gin.Context, capture *ImageErrorEvidence) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if c.GetInt("id") > 0 {
		capture.Index.UserID = c.GetInt("id")
	}
	if c.GetInt("token_id") > 0 {
		capture.Index.TokenID = c.GetInt("token_id")
		capture.Index.AppID = c.GetInt("token_id")
	}
	if c.GetInt("app_id") > 0 {
		capture.Index.AppID = c.GetInt("app_id")
	}
	if c.GetInt("channel_id") > 0 {
		capture.Index.ChannelID = c.GetInt("channel_id")
	}
	if modelName := c.GetString("original_model"); modelName != "" {
		capture.Index.ClientModel = modelName
	}
	for _, param := range []string{"task_id", "video_id"} {
		if id := c.Param(param); id != "" {
			capture.Index.TaskID = id
			break
		}
	}
}

func newAPIRequestEvidence(index model.TaskRequestEvidence) *ImageErrorEvidence {
	if !evidenceEnabled() {
		return nil
	}
	if index.CreatedAt == 0 {
		index.CreatedAt = common.GetTimestamp()
	}
	return &ImageErrorEvidence{Index: index, worker: int(evidenceWorkerSequence.Add(1) % evidenceWorkerCount), records: make(map[*imageEvidenceExchange]struct{})}
}
func apiEvidenceFrom(ctx context.Context) *ImageErrorEvidence {
	if ctx == nil {
		return nil
	}
	capture, _ := ctx.Value(apiRequestEvidenceKey{}).(*ImageErrorEvidence)
	return capture
}
func HasAPIRequestEvidence(ctx context.Context) bool { return apiEvidenceFrom(ctx) != nil }

func BindAPIRequestEvidence(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) {
	if c == nil || c.Request == nil || req == nil {
		return
	}
	capture := apiEvidenceFrom(c.Request.Context())
	if capture == nil {
		return
	}
	updateRequestEvidenceIdentity(c, capture)
	*req = *req.WithContext(context.WithValue(req.Context(), apiRequestEvidenceKey{}, capture))
	if info != nil {
		capture.mu.Lock()
		capture.Index.ChannelID = info.ChannelId
		capture.Index.ClientModel = info.OriginModelName
		capture.Index.UpstreamModel = info.UpstreamModelName
		capture.mu.Unlock()
	}
}

// Only external operation/headers decide file exclusion, before the first copy.
// JSON is opaque, including embedded data URLs; there is no late body decoder.
func evidenceFileBody(method, path string, headers http.Header) bool {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(headers.Get("Content-Type"), ";")[0]))
	if (method == http.MethodPost && (path == "/v1/files" || strings.HasSuffix(path, "/files"))) ||
		(method == http.MethodPost && strings.HasSuffix(path, "/submit/upload-discord-images")) ||
		strings.HasSuffix(path, "/content") || strings.Contains(path, "/mj/image/") {
		return true
	}
	if strings.HasPrefix(contentType, "multipart/") || strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "audio/") || strings.HasPrefix(contentType, "video/") {
		return true
	}
	if contentType == "application/octet-stream" || contentType == "application/jsonl" || contentType == "application/x-ndjson" || contentType == "application/pdf" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(headers.Get("Content-Disposition")), "attachment")
}
func (c *ImageErrorEvidence) requestRecord(req *http.Request, stage string) *imageEvidenceExchange {
	path := ""
	rawQuery := ""
	if req.URL != nil {
		path = req.URL.Path
		rawQuery = req.URL.RawQuery
	}
	// Response bodies are independently eligible even on an upload endpoint.
	skip := evidenceFileBody(req.Method, path, req.Header)
	return c.newRecord(stage, EvidenceObjectMeta{Method: req.Method, Target: evidenceTargetURL(req), Headers: req.Header, RawQuery: rawQuery, ContentType: req.Header.Get("Content-Type")}, skip)
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
	if capture == nil || evidenceInfrastructureRequest(req) || evidenceDiskPressure.Load() {
		return t.RoundTripper.RoundTrip(req)
	}
	record := capture.requestRecord(req, model.TaskRequestEvidenceStageSouthboundSend)
	if req.Body != nil {
		req.Body = &imageEvidenceBody{ReadCloser: req.Body, capture: capture, record: record}
	} else {
		record.close()
	}
	resp, err := t.RoundTripper.RoundTrip(req)
	capture.observeResponse(req, resp, err, model.TaskRequestEvidenceStageUpstreamResponse)
	return resp, err
}
func (c *ImageErrorEvidence) observeResponse(req *http.Request, resp *http.Response, transportErr error, stage string) {
	meta := EvidenceObjectMeta{}
	path := ""
	if req != nil {
		meta.Method = req.Method
		meta.Target = evidenceTargetURL(req)
		if req.URL != nil {
			path = req.URL.Path
			meta.RawQuery = req.URL.RawQuery
		}
	}
	if transportErr != nil {
		meta.Error = "transport_error"
	}
	skip := false
	if resp != nil {
		meta.StatusCode = resp.StatusCode
		meta.Headers = resp.Header
		meta.ContentType = resp.Header.Get("Content-Type")
		skip = evidenceFileBody("", path, resp.Header)
	}
	record := c.newRecord(stage, meta, skip)
	if resp != nil && resp.Body != nil {
		resp.Body = &imageEvidenceBody{ReadCloser: resp.Body, capture: c, record: record}
	} else {
		record.close()
	}
}
func (t *apiEvidenceTransport) CloseIdleConnections() {
	if closer, ok := t.RoundTripper.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// Arbitrary URL/OSS fetches explicitly remove the carrier at their own boundary.
type evidenceInfrastructureKey struct{}

func WithoutRequestEvidence(ctx context.Context) context.Context {
	return context.WithValue(ctx, evidenceInfrastructureKey{}, true)
}
func evidenceInfrastructureRequest(req *http.Request) bool {
	disabled, _ := req.Context().Value(evidenceInfrastructureKey{}).(bool)
	return disabled
}

func evidenceIndexForTask(task *model.Task) model.TaskRequestEvidence {
	return model.TaskRequestEvidence{RequestID: common.GetUUID(), UserID: task.UserId, AppID: task.AppID, TaskID: task.TaskID, ChannelID: task.ChannelId, ClientModel: task.Properties.OriginModelName, ClientProtocol: task.ClientProtocol, Kind: "task_request", CreatedAt: common.GetTimestamp()}
}
func BeginBackgroundRequestEvidence(ctx context.Context, task *model.Task) (context.Context, func()) {
	if !evidenceEnabled() || evidenceDiskPressure.Load() || task == nil || apiEvidenceFrom(ctx) != nil {
		return ctx, func() {}
	}
	capture := newAPIRequestEvidence(evidenceIndexForTask(task))
	return context.WithValue(ctx, apiRequestEvidenceKey{}, capture), func() { capture.Finish(false) }
}
func BeginBatchRequestEvidence(ctx context.Context, job *model.BatchJob) (context.Context, func()) {
	if !evidenceEnabled() || evidenceDiskPressure.Load() || job == nil || apiEvidenceFrom(ctx) != nil {
		return ctx, func() {}
	}
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: common.GetUUID(), UserID: job.UserId, AppID: job.AppID, TaskID: job.Id, Kind: "batch_request"})
	return context.WithValue(ctx, apiRequestEvidenceKey{}, capture), func() { capture.Finish(false) }
}

// Header metadata is bounded separately from the opaque body. Oversized
// metadata is omitted as a resource failure, never copied then inspected.
func cloneEvidenceHeaders(headers http.Header) http.Header {
	size := 0
	for key, values := range headers {
		size += len(key)
		for _, value := range values {
			size += len(value)
		}
		if size > 64<<10 {
			imageEvidenceDropped.Add(1)
			return nil
		}
	}
	return headers.Clone()
}
