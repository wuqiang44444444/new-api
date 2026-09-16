// Package testutil provides an isolated asynchronous event collector for contract
// tests. It never swaps the process-wide logger or introduces a synchronous mode.
package testutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/stretchr/testify/require"
)

type Capture struct {
	*clienterrlog.Sink
	t      *testing.T
	events chan clienterrlog.Event
	count  uint64
	buffer strings.Builder
}

func New(t *testing.T) *Capture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Capture{t: t, events: make(chan clienterrlog.Event, 1024)}
	c.Sink = clienterrlog.NewSink(ctx, 1024, func(e clienterrlog.Event) error { c.events <- e; return nil })
	return c
}

// String waits only for events already accepted before this call. A zero-event
// assertion uses the synchronous acceptance count, not a timing-based absence.
func (c *Capture) String() string {
	c.t.Helper()
	accepted := c.Health().Accepted
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for c.count < accepted {
		select {
		case e := <-c.events:
			c.buffer.WriteString(e.Message)
			c.buffer.WriteByte('\n')
			c.count++
		case <-timer.C:
			c.t.Fatal("accepted client error event was not delivered")
		}
	}
	require.Zero(c.t, c.Health().Dropped)
	return c.buffer.String()
}
func (c *Capture) Len() int { return len(c.String()) }
