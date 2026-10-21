package service

import (
	"context"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func DialRealtimeWithEvidence(c *gin.Context, target string, headers http.Header) (*websocket.Conn, error) {
	conn, _, err := DialRealtimeWithEvidenceUsing(c, websocket.DefaultDialer, context.Background(), target, headers)
	return conn, err
}

func DialRealtimeWithEvidenceUsing(c *gin.Context, dialer *websocket.Dialer, ctx context.Context, target string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	capture := apiEvidenceFrom(c.Request.Context())
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, err
	}
	if capture != nil {
		req.Header = headers
		capture.requestRecord(req, model.TaskRequestEvidenceStageSouthboundSend)
	}
	conn, resp, err := dialer.DialContext(ctx, target, headers)
	if capture != nil {
		record := &imageEvidenceExchange{Stage: model.TaskRequestEvidenceStageUpstreamResponse, Method: http.MethodGet, Target: evidenceTargetURL(req), CreatedAt: common.GetTimestamp()}
		if err != nil {
			record.Error = "handshake_error"
		}
		if resp != nil {
			record.Status = resp.StatusCode
			record.Headers = resp.Header.Clone()
			record.ContentType = resp.Header.Get("Content-Type")
			if err != nil && resp.Body != nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, evidenceMaxBodyBytes()+1))
				_ = resp.Body.Close()
				capture.mu.Lock()
				capture.appendBody(record, body)
				capture.mu.Unlock()
				if readErr != nil {
					record.Error = readErr.Error()
				}
			}
		}
		capture.mu.Lock()
		capture.records = append(capture.records, record)
		capture.mu.Unlock()
	}
	return conn, resp, err
}

// Frames use separate stages from HTTP delivery, retaining the handshake's real
// HTTP status rather than treating a later realtime failure as HTTP 500.
func ObserveRealtimeFrame(ctx context.Context, stage string, message []byte, err error) {
	capture := apiEvidenceFrom(ctx)
	if capture == nil {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.finished {
		return
	}
	var record *imageEvidenceExchange
	for _, item := range capture.records {
		if item.Stage == stage {
			record = item
			break
		}
	}
	if record == nil {
		record = &imageEvidenceExchange{Stage: stage, ContentType: "text/event-stream", CreatedAt: common.GetTimestamp()}
		capture.records = append(capture.records, record)
	}
	if len(message) > 0 {
		capture.appendBody(record, append(append([]byte("data: "), message...), '\n'))
	}
	if err != nil {
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			record.Error = "realtime_io_error"
		}
		return
	}
}
