package service

import (
	"context"
	"net/http"

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
	if capture != nil {
		updateRequestEvidenceIdentity(c, capture)
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			return nil, nil, err
		}
		req.Header = headers
		capture.requestRecord(req, model.TaskRequestEvidenceStageSouthboundSend).close()
	}
	conn, resp, err := dialer.DialContext(ctx, target, headers)
	if capture != nil {
		meta := EvidenceObjectMeta{Method: http.MethodGet}
		if resp != nil {
			meta.StatusCode = resp.StatusCode
			meta.Headers = resp.Header
		}
		if err != nil {
			meta.Error = "handshake_error"
		}
		capture.newRecord(model.TaskRequestEvidenceStageUpstreamResponse, meta, false).close()
	}
	return conn, resp, err
}

// A direction shares one bounded stream, sealed every fifteen seconds or at close.
func ObserveRealtimeFrame(ctx context.Context, stage string, messageType int, message []byte, frameErr error) {
	capture := apiEvidenceFrom(ctx)
	if capture != nil {
		capture.realtimeRecord(stage).appendRealtimeMessage(messageType, message, frameErr)
	}
}

func WriteRealtimeFrameWithEvidence(ctx context.Context, stage string, conn *websocket.Conn, messageType int, message []byte) error {
	err := conn.WriteMessage(messageType, message)
	if err == nil {
		ObserveRealtimeFrame(ctx, stage, messageType, message, nil)
	} else {
		ObserveRealtimeFrame(ctx, stage, messageType, nil, err)
	}
	return err
}
