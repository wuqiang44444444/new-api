package service

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Transport-outcome classification for Link video create attempts.
// Contract: docs/80-dev/2026-09-22-视频创建未知结果长期占款问题分析与退款闭环方案.md §3.1.
// Only a provably never-sent call may take the verified rejection release
// path; anything else stays unknown and keeps the current semantics. The
// classification must run before both unknown-marking sites.

type taskCreateTransportProbeState struct {
	dialAttempted    atomic.Uint32
	dialFailures     atomic.Uint32
	connsEstablished atomic.Uint32
	wroteAny         atomic.Bool
}

// AttachTaskCreateTransportProbe installs a per-request probe transport when a
// Link video create attempt is active. It returns the (possibly) rewritten
// request. Without an active attempt it is a no-op, so native relay behavior
// is untouched.
func AttachTaskCreateTransportProbe(c *gin.Context, req *http.Request, client *http.Client) *http.Request {
	if c == nil || req == nil || client == nil {
		return req
	}
	if int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID)) == 0 {
		return req
	}
	state := &taskCreateTransportProbeState{}
	probe, ok := newTaskCreateTransportProbe(client.Transport, state)
	if !ok {
		// Unwrappable transport: stay conservative (unknown) instead of
		// guessing "not sent".
		return req
	}
	client.Transport = probe
	common.SetContextKey(c, constant.ContextKeyTaskCreateTransportProbe, state)
	return req
}

// newTaskCreateTransportProbe clones the base transport and wraps its dial
// path so connection establishment and request writes become observable for
// exactly this request. The clone gets a fresh connection pool: Link video
// creates are low-QPS single-POST operations, so the lost cross-request reuse
// is the accepted cost of exact per-request attribution.
func newTaskCreateTransportProbe(base http.RoundTripper, state *taskCreateTransportProbeState) (http.RoundTripper, bool) {
	transport, ok := base.(*http.Transport)
	if !ok {
		return nil, false
	}
	clone := transport.Clone()
	clone.DisableKeepAlives = true // This per-call pool must not retain idle sockets.
	innerDial := clone.DialContext
	if innerDial == nil {
		defaultDialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		innerDial = defaultDialer.DialContext
	}
	clone.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		state.dialAttempted.Add(1)
		conn, err := innerDial(ctx, network, address)
		if err != nil {
			state.dialFailures.Add(1)
			return nil, err
		}
		state.connsEstablished.Add(1)
		return &taskCreateProbeConn{Conn: conn, state: state}, nil
	}
	return clone, true
}

// taskCreateProbeConn observes whether any bytes were written on connections
// created for this request (request body, CONNECT payload, transparent
// transport retries, partial writes).
type taskCreateProbeConn struct {
	net.Conn
	state *taskCreateTransportProbeState
}

func (c *taskCreateProbeConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.state.wroteAny.Store(true)
	}
	return n, err
}

// provenUnsent returns true only when the probe proves the whole call never
// reached a connection carrying request bytes: at least one dial attempt, no
// established connection, and zero bytes written anywhere in the call.
func (s *taskCreateTransportProbeState) provenUnsent() bool {
	return s != nil &&
		s.dialAttempted.Load() > 0 &&
		s.connsEstablished.Load() == 0 &&
		!s.wroteAny.Load()
}

// provenSent reports whether any connection for this request carried bytes.
func (s *taskCreateTransportProbeState) provenSent() bool {
	return s != nil && s.connsEstablished.Load() > 0 && s.wroteAny.Load()
}

// taskCreateTransportProbeStateFromGin reads the probe state recorded on the
// gin context. The outbound request is locally rebound in doRequest, so
// c.Request.Context() is not the owner of the probe state.
func taskCreateTransportProbeStateFromGin(c *gin.Context) *taskCreateTransportProbeState {
	if c == nil {
		return nil
	}
	state, _ := c.Get(string(constant.ContextKeyTaskCreateTransportProbe))
	probeState, _ := state.(*taskCreateTransportProbeState)
	return probeState
}

// SettleTaskCreateTransportOutcome classifies a client.Do transport error at
// the Link create boundary before both unknown-marking sites. A provably
// never-sent call takes the existing terminal-rejection release path
// (controller releases the hold after the no-retry Link loop); anything else
// keeps the current unknown semantics untouched.
func SettleTaskCreateTransportOutcome(c *gin.Context, info *relaycommon.RelayInfo, err error) {
	if c == nil || info == nil || err == nil {
		return
	}
	if int64(common.GetContextKeyInt(c, constant.ContextKeyTaskCreateAttemptID)) == 0 {
		return
	}
	if taskCreateTransportProbeStateFromGin(c).provenUnsent() {
		relaycommon.SetTaskCreateDisposition(c, relaycommon.TaskCreateTerminalRejection)
		attachTaskCreateTransportDiagnostics(c, info, "transport_unsent_released")
		return
	}
	attachTaskCreateTransportDiagnostics(c, info, "transport_outcome_unknown")
	MarkTaskCreateAttemptOutcomeUnknown(c, info)
}

func attachTaskCreateTransportDiagnostics(c *gin.Context, info *relaycommon.RelayInfo, reason string) {
	if c == nil || info == nil {
		return
	}
	channelID := 0
	if info.ChannelMeta != nil {
		channelID = info.ChannelId
	}
	protocol := ""
	if info.TaskRelayInfo != nil {
		protocol = string(info.ClientProtocol)
	}
	clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
		Stage:     "task_create_transport",
		Reason:    clienterrlog.SanitizeLogValue(reason, 64),
		Model:     clienterrlog.SanitizeLogValue(info.OriginModelName, 128),
		ChannelID: channelID,
		Protocol:  clienterrlog.SanitizeLogValue(protocol, 64),
	})
}
