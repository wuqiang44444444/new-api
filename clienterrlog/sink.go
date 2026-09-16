package clienterrlog

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/logger"
)

// Event is an immutable, bounded snapshot. It never retains a request or Context.
type Event struct {
	At        time.Time
	RequestID string
	Message   string
}

// Sink owns one bounded queue and one writer. A blocked writer cannot delay
// submission or create additional goroutines. NewSink permits isolated outputs
// for contract tests; the application uses only the process-wide default sink.
type Sink struct {
	queue        chan Event
	write        func(Event) error
	available    atomic.Bool
	accepted     atomic.Uint64
	dropped      atomic.Uint64
	written      atomic.Uint64
	failed       atomic.Uint64
	writeStarted atomic.Int64
	lastSuccess  atomic.Int64
}

var diagnosticFailures atomic.Uint64
var defaultSink = NewSink(context.Background(), 1024, func(e Event) error {
	return logger.WriteClientErrorEvent(e.At, e.RequestID, e.Message)
})

// Health is process-local delivery evidence, independent of the logging output.
// accepted = written + failed + queued + in-flight once concurrent updates settle.
// Drops and failures are cumulative; an in-flight age exposes a blocked output.
type Health struct {
	Available          bool   `json:"available"`
	Capacity           int    `json:"capacity"`
	Queued             int    `json:"queued"`
	Accepted           uint64 `json:"accepted"`
	Dropped            uint64 `json:"dropped"`
	Written            uint64 `json:"written"`
	Failed             uint64 `json:"failed"`
	DiagnosticFailures uint64 `json:"diagnostic_failures"`
	WriteStartedAt     int64  `json:"write_started_at"`
	LastSuccessAt      int64  `json:"last_success_at"`
}

func NewSink(ctx context.Context, capacity int, write func(Event) error) (s *Sink) {
	s = &Sink{write: write}
	defer func() {
		if recover() != nil {
			s.available.Store(false)
			diagnosticFailures.Add(1)
		}
	}()
	if ctx == nil || capacity <= 0 || write == nil {
		return s
	}
	s.queue = make(chan Event, capacity)
	s.available.Store(true)
	go s.run(ctx)
	return s
}

func (s *Sink) Health() Health {
	if s == nil {
		return Health{DiagnosticFailures: diagnosticFailures.Load()}
	}
	return Health{Available: s.available.Load(), Capacity: cap(s.queue), Queued: len(s.queue), Accepted: s.accepted.Load(), Dropped: s.dropped.Load(), Written: s.written.Load(), Failed: s.failed.Load(), DiagnosticFailures: diagnosticFailures.Load(), WriteStartedAt: s.writeStarted.Load(), LastSuccessAt: s.lastSuccess.Load()}
}

func CurrentHealth() Health { return defaultSink.Health() }

func (s *Sink) submit(e Event) {
	if s == nil {
		diagnosticFailures.Add(1)
		return
	}
	if !s.available.Load() {
		s.dropped.Add(1)
		return
	}
	select {
	case s.queue <- e:
		s.accepted.Add(1)
	default:
		s.dropped.Add(1)
	}
}

// Submit enqueues an event without ever blocking; drops are counted on the
// instance's own counters. Exported for independent event producers that own
// their Sink instance (for example image task execution diagnostics) so they
// keep separate loss statistics from the request-scoped 4xx recorder.
func (s *Sink) Submit(e Event) {
	s.submit(e)
}

func (s *Sink) run(ctx context.Context) {
	defer s.available.Store(false)
	for {
		select {
		case <-ctx.Done():
			return // Never wait for drain at shutdown.
		case e := <-s.queue:
			s.deliver(e)
		}
	}
}

func (s *Sink) deliver(e Event) {
	s.writeStarted.Store(time.Now().Unix())
	defer s.writeStarted.Store(0)
	defer func() {
		if recover() != nil {
			s.failed.Add(1)
		}
	}()
	if err := s.write(e); err != nil {
		s.failed.Add(1)
		return
	}
	s.written.Add(1)
	s.lastSuccess.Store(time.Now().Unix())
}

// Only logging operations use this guard. It must never surround c.Next().
func isolateDiagnosticPanic() {
	if recover() != nil {
		diagnosticFailures.Add(1)
	}
}
