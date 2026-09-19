package controller

import (
	"context"
	"net/http/httptrace"
	"sync/atomic"
)

// Observe transport attempts separately from received response bytes. Neither
// an attempted connection nor a missing response proves provider acceptance.
type channelTestNetwork struct {
	attempted atomic.Bool
	received  atomic.Bool
}

func observeChannelTestNetwork(ctx context.Context) (context.Context, *channelTestNetwork) {
	observation := &channelTestNetwork{}
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GetConn:              func(string) { observation.attempted.Store(true) },
		GotFirstResponseByte: func() { observation.received.Store(true) },
	}), observation
}
