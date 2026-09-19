package clienterrlog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// HTTP diagnostics are admin-only, sanitized snapshots, never replay material.
// Oversized or malformed bodies are omitted rather than storing unsafe prefixes.
const HTTPExchangeDetailKey = "http_exchange"
const diagnosticBodyLimit = 16 * 1024

type HTTPBody struct {
	State  string `json:"state"`
	Body   string `json:"body,omitempty"`
	Status int    `json:"status,omitempty"`
}

type HTTPExchange struct {
	Request          HTTPBody `json:"request"`
	UpstreamRequest  HTTPBody `json:"upstream_request"`
	UpstreamResponse HTTPBody `json:"upstream_response"`
	Response         HTTPBody `json:"response"`
}

type exchangeContextKey struct{}
type bodyObservation struct {
	data      []byte
	seen      bool
	oversized bool
	failed    bool
	status    int
}
type exchangeCapture struct {
	mu     sync.Mutex
	bodies map[string]*bodyObservation
}

// InstallHTTPExchange observes bytes already read/written by the handler. It
// never drains a body, changes headers, or delays forwarding for diagnostics.
func InstallHTTPExchange(c *gin.Context) {
	capture := &exchangeCapture{bodies: make(map[string]*bodyObservation)}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), exchangeContextKey{}, capture))
	if c.Request.Body != nil {
		c.Request.Body = &diagnosticBodyReader{ReadCloser: c.Request.Body, ctx: c.Request.Context(), kind: "request"}
	}
	c.Writer = &diagnosticResponseWriter{ResponseWriter: c.Writer, ctx: c.Request.Context()}
}

func ObserveHTTPBody(ctx context.Context, kind string, data []byte, status int) {
	capture, ok := ctx.Value(exchangeContextKey{}).(*exchangeCapture)
	if !ok {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	body := capture.bodies[kind]
	if body == nil {
		body = &bodyObservation{}
		capture.bodies[kind] = body
	}
	body.seen = true
	if status > 0 {
		body.status = status
	}
	if body.oversized {
		return
	}
	if len(body.data)+len(data) > diagnosticBodyLimit {
		body.oversized = true
		body.data = nil
		return
	}
	body.data = append(body.data, data...)
}

// EnsureUpstreamResponseCapture avoids recording twice when a channel test
// already observes the same response at the adapter boundary.
func EnsureUpstreamResponseCapture(ctx context.Context, resp *http.Response) {
	if _, wrapped := resp.Body.(*diagnosticBodyReader); wrapped {
		return
	}
	WrapUpstreamResponse(ctx, resp)
}

// WrapUpstreamRequest leaves ContentLength, GetBody and all routing/transport
// settings intact. It observes only bytes the transport actually consumes.
func WrapUpstreamRequest(ctx context.Context, req *http.Request) {
	// Native adaptors may construct requests without the incoming context.
	// Propagate an installed diagnostic trace without changing cancellation.
	if req != nil {
		if trace := httptrace.ContextClientTrace(ctx); trace != nil && httptrace.ContextClientTrace(req.Context()) != trace {
			*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
		}
	}
	capture, ok := ctx.Value(exchangeContextKey{}).(*exchangeCapture)
	if !ok || req == nil || req.Body == nil {
		return
	}
	capture.mu.Lock()
	capture.bodies["upstream_request"] = &bodyObservation{}
	delete(capture.bodies, "upstream_response")
	capture.mu.Unlock()
	req.Body = &diagnosticBodyReader{ReadCloser: req.Body, ctx: ctx, kind: "upstream_request"}
}

func WrapUpstreamResponse(ctx context.Context, resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if _, ok := ctx.Value(exchangeContextKey{}).(*exchangeCapture); !ok {
		return
	}
	capture := ctx.Value(exchangeContextKey{}).(*exchangeCapture)
	capture.mu.Lock()
	capture.bodies["upstream_response"] = &bodyObservation{seen: true, status: resp.StatusCode}
	capture.mu.Unlock()
	resp.Body = &diagnosticBodyReader{ReadCloser: resp.Body, ctx: ctx, kind: "upstream_response"}
}

type diagnosticBodyReader struct {
	io.ReadCloser
	ctx  context.Context
	kind string
}

func (r *diagnosticBodyReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	ObserveHTTPBody(r.ctx, r.kind, p[:n], 0)
	if err != nil && err != io.EOF {
		capture := r.ctx.Value(exchangeContextKey{}).(*exchangeCapture)
		capture.mu.Lock()
		capture.bodies[r.kind].failed = true
		capture.mu.Unlock()
	}
	return n, err
}

type diagnosticResponseWriter struct {
	gin.ResponseWriter
	ctx context.Context
}

func (w *diagnosticResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	ObserveHTTPBody(w.ctx, "response", p[:n], w.Status())
	return n, err
}
func (w *diagnosticResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func SnapshotHTTPExchange(ctx context.Context) *HTTPExchange {
	if ctx == nil {
		return nil
	}
	capture, ok := ctx.Value(exchangeContextKey{}).(*exchangeCapture)
	if !ok {
		return nil
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return &HTTPExchange{
		Request:          sanitizeHTTPBody(capture.bodies["request"]),
		UpstreamRequest:  sanitizeHTTPBody(capture.bodies["upstream_request"]),
		UpstreamResponse: sanitizeHTTPBody(capture.bodies["upstream_response"]),
		Response:         sanitizeHTTPBody(capture.bodies["response"]),
	}
}

func sanitizeHTTPBody(body *bodyObservation) HTTPBody {
	result := HTTPBody{State: "not_recorded"}
	if body == nil || !body.seen {
		return result
	}
	result.Status = body.status
	switch {
	case body.failed:
		result.State = "read_failed"
	case body.oversized:
		result.State = "too_large"
	case len(body.data) == 0:
		result.State = "empty"
	default:
		sanitized, err := redactDiagnosticJSON(body.data, 0)
		if err != nil {
			result.State = "unsupported"
			return result
		}
		sanitized, err = common.IndentJson(sanitized)
		if err != nil {
			result.State = "unsupported"
			return result
		}
		if len(sanitized) > diagnosticBodyLimit {
			result.State = "too_large"
			return result
		}
		result.State, result.Body = "captured", string(sanitized)
		// Detail is a MySQL TEXT column. The exchange itself is a JSON string
		// inside Detail, so budget both escaping layers, not just input bytes.
		// Four 12 KiB sections leave room for the bounded diagnostic metadata.
		encoded, err := common.Marshal(result)
		if err == nil {
			encoded, err = common.Marshal(string(encoded))
		}
		if err != nil {
			return HTTPBody{State: "unsupported", Status: body.status}
		}
		if len(encoded) > 12*1024 {
			return HTTPBody{State: "too_large", Status: body.status}
		}
	}
	return result
}

// Recursing through RawMessage preserves exact numbers (including large IDs),
// nulls, and explicit zero/false values. Free text gets the established technical
// diagnostic sanitizer; URLs, binary content, and credential fields are removed.
func redactDiagnosticJSON(data []byte, depth int) ([]byte, error) {
	if depth > 32 {
		return nil, io.ErrUnexpectedEOF
	}
	var raw json.RawMessage
	if err := common.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	switch common.GetJsonType(raw) {
	case "object":
		var object map[string]json.RawMessage
		if err := common.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		for key, value := range object {
			if diagnosticSensitiveKey(key) {
				object[key] = json.RawMessage(`"[REDACTED]"`)
				continue
			}
			sanitized, err := redactDiagnosticJSON(value, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = sanitized
		}
		return common.Marshal(object)
	case "array":
		var items []json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for i, item := range items {
			sanitized, err := redactDiagnosticJSON(item, depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = sanitized
		}
		return common.Marshal(items)
	case "string":
		var value string
		if err := common.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		if strings.Contains(strings.ToLower(value), "data:") {
			return common.Marshal("[REDACTED]")
		}
		return common.Marshal(common.SanitizeTaskDiagnostic(value))
	default:
		return raw, nil
	}
}

func diagnosticSensitiveKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(key))
	switch key {
	case "token", "authorization", "proxyauthorization", "cookie", "setcookie", "key", "apikey", "accesskey", "accesskeyid", "secretaccesskey", "secretkey", "sessionkey", "privatekey", "password", "passwd", "credential", "credentials", "signature", "payment", "paymentcredential", "cardnumber", "cvv", "url", "source", "sourceurl", "b64json", "base64", "imagedata", "audiodata", "inlinedata", "inputaudio", "image", "images", "audio", "video", "filedata", "privatedata", "taskprivatedata":
		return true
	}
	for _, suffix := range []string{"token", "secret", "password", "apikey", "credential", "signature", "url"} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}
