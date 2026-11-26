package service

import (
	"context"
	"net/http"
	"sync/atomic"
)

type textResponseAcceptanceKey struct{}

const (
	textResponseWaiting uint32 = iota
	textResponseAccepted
	textResponseCanceled
)

// DoSynchronousTextRequest lets downstream cancellation stop an outbound
// request until the transport accepts the response. The accepted body keeps
// the original outbound deadline and remains readable for usage settlement.
func DoSynchronousTextRequest(client *http.Client, req *http.Request, downstream context.Context) (*http.Response, error) {
	if err := downstream.Err(); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}

	var acceptance atomic.Uint32
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(downstream, func() {
		if acceptance.CompareAndSwap(textResponseWaiting, textResponseCanceled) {
			cancel()
		}
	})
	defer stop()
	ctx = context.WithValue(ctx, textResponseAcceptanceKey{}, &acceptance)
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		cancel()
		return resp, err
	}
	// The client may use an injected transport without the relay's hook.
	if !acceptTextResponse(&acceptance) {
		_ = resp.Body.Close()
		cancel()
		return nil, downstream.Err()
	}
	OnUpstreamResponseBodyClose(resp, cancel)
	return resp, nil
}

func acceptTextResponse(acceptance *atomic.Uint32) bool {
	return acceptance.CompareAndSwap(textResponseWaiting, textResponseAccepted) ||
		acceptance.Load() == textResponseAccepted
}

// Mark acceptance before evidence capture or the HTTP client can delay handoff.
func roundTripWithTextResponseAcceptance(transport http.RoundTripper, req *http.Request) (*http.Response, error) {
	resp, err := transport.RoundTrip(req)
	if err == nil && resp != nil {
		if acceptance, _ := req.Context().Value(textResponseAcceptanceKey{}).(*atomic.Uint32); acceptance != nil {
			acceptTextResponse(acceptance)
		}
	}
	return resp, err
}
