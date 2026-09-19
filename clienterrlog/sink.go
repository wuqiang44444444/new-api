package clienterrlog

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/logger"
)

// Event is an immutable, bounded snapshot. It never retains a request or Context.
// Message carries the single-line WARN payload; the structured fields are
// consumed only by the registered event persister (model/error_event.go).
type Event struct {
	At                time.Time
	RequestID         string
	Message           string
	EventType         string
	Module            string
	Method            string
	Route             string
	Status            int
	UserID            int
	Username          string
	TokenName         string
	Stage             string
	Reason            string
	PublicCode        string
	Model             string
	ChannelID         int
	TaskID            string
	Protocol          string
	UpstreamRequestID string
	ElapsedMs         int64
	Detail            map[string]string
	HTTPExchange      *HTTPExchange
}

// persistedModules lists the route modules whose API-call errors are persisted
// to the error_events table (token gateway traffic: relay + asset). Dashboard
// ("api"/"old_api") and static web routes stay log-line only by contract.
var persistedModules = map[string]bool{"relay": true, "asset": true}

var eventPersister atomic.Value // stores *func(Event) error

// SetEventPersister registers the durable persistence outlet for API error
// events. Nil disables persistence; it never affects the log-line outlet.
func SetEventPersister(persist func(Event) error) {
	if persist == nil {
		eventPersister.Store((*func(Event) error)(nil))
		return
	}
	eventPersister.Store(&persist)
}

func loadEventPersister() func(Event) error {
	if stored, ok := eventPersister.Load().(*func(Event) error); ok && stored != nil {
		return *stored
	}
	return nil
}

// Sink owns one bounded queue and one writer. A blocked writer cannot delay
// submission or create additional goroutines. NewSink permits isolated outputs
// for contract tests; the application uses only the process-wide default sink.
type Sink struct {
	queue         chan Event
	write         func(Event) error
	available     atomic.Bool
	accepted      atomic.Uint64
	dropped       atomic.Uint64
	written       atomic.Uint64
	failed        atomic.Uint64
	persisted     atomic.Uint64
	persistFailed atomic.Uint64
	writeStarted  atomic.Int64
	lastSuccess   atomic.Int64
}

var diagnosticFailures atomic.Uint64
var defaultSink = NewSink(context.Background(), 1024, func(e Event) error {
	return logger.WriteClientErrorEvent(e.At, e.RequestID, e.Message)
})

// Health is process-local delivery evidence, independent of the logging output.
// accepted counts submissions; written/failed and persisted/persist_failed count
// per-outlet delivery attempts, and an api_error event may attempt both outlets
// (a failed WARN write no longer blocks persistence). Drops and failures are
// cumulative; an in-flight age exposes a blocked output.
type Health struct {
	Available          bool   `json:"available"`
	Capacity           int    `json:"capacity"`
	Queued             int    `json:"queued"`
	Accepted           uint64 `json:"accepted"`
	Dropped            uint64 `json:"dropped"`
	Written            uint64 `json:"written"`
	Failed             uint64 `json:"failed"`
	Persisted          uint64 `json:"persisted"`
	PersistFailed      uint64 `json:"persist_failed"`
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
	return Health{Available: s.available.Load(), Capacity: cap(s.queue), Queued: len(s.queue), Accepted: s.accepted.Load(), Dropped: s.dropped.Load(), Written: s.written.Load(), Failed: s.failed.Load(), Persisted: s.persisted.Load(), PersistFailed: s.persistFailed.Load(), DiagnosticFailures: diagnosticFailures.Load(), WriteStartedAt: s.writeStarted.Load(), LastSuccessAt: s.lastSuccess.Load()}
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
	// The WARN outlet stays a 4xx diagnostic channel: legacy producers without
	// an event type and request-scoped api_error events below 500. Stream,
	// channel-test and task events never reach the log writer, so the native
	// WARN 4xx statistics stay untouched by the extended sources.
	if isWarnEligible(e) && (e.Status == 0 || e.Status < 500) {
		s.writeWARN(e)
	}
	// A failed WARN write must not block persistence; the two outlets stay
	// independent, so there is deliberately no early return after write errors.
	if persist := loadEventPersister(); persist != nil && persistedModules[e.Module] {
		s.persistEvent(persist, e)
	}
}

// writeWARN isolates log-writer failures so persistence is attempted even when
// the writer panics. Each outlet owns its own failure counter and recovery.
func (s *Sink) writeWARN(e Event) {
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

// isWarnEligible reports whether the event may reach the WARN log-line outlet.
func isWarnEligible(e Event) bool {
	return e.EventType == "" || e.EventType == EventAPIError
}

// persistEvent isolates persister failures from the WARN outlet; a failed or
// panicking database write never changes the log-line delivery result.
func (s *Sink) persistEvent(persist func(Event) error, e Event) {
	defer func() {
		if recover() != nil {
			s.persistFailed.Add(1)
		}
	}()
	if err := persist(e); err != nil {
		s.persistFailed.Add(1)
		return
	}
	s.persisted.Add(1)
}

// Only logging operations use this guard. It must never surround c.Next().
func isolateDiagnosticPanic() {
	if recover() != nil {
		diagnosticFailures.Add(1)
	}
}
