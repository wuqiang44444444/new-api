package service

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type imageObjectSessionKey struct{}

// A session pins one immutable client and its node-local readiness observation.
// It is not a task, storage configuration, or durable source of truth.
type imageObjectSession struct {
	store     imageObjectStore
	mu        sync.Mutex
	running   chan struct{}
	expires   time.Time
	err       error
	healthKey string
}

func (h *taskArtifactStoreRuntimeHolder) getImageSession() (*imageObjectSession, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.imageSession == nil {
		store, ok := h.store.(imageObjectStore)
		if !ok {
			return nil, ErrTaskArtifactStoreDisabled
		}
		h.imageSession = &imageObjectSession{store: store, healthKey: "object-storage-health/" + common.GetUUID()}
	}
	return h.imageSession, nil
}

func imageObjectSessionForContext(ctx context.Context) (*imageObjectSession, error) {
	if session, ok := ctx.Value(imageObjectSessionKey{}).(*imageObjectSession); ok {
		return session, nil
	}
	return taskArtifactStoreRuntime.getImageSession()
}

// WithImageObjectStore binds the same client across upload/read/HEAD/signing.
// Nested operations preserve the caller's binding even during credential rotation.
func WithImageObjectStore(ctx context.Context) (context.Context, error) {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, imageObjectSessionKey{}, session), nil
}

// CheckImageObjectStoreReady checks current write/read ability, not future availability.
// Concurrent callers share one probe; success is cached for 10s, failure for 2s.
// The small non-customer object is reused per client instance; bucket lifecycle owns cleanup.
func CheckImageObjectStoreReady(ctx context.Context) error {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		session.mu.Lock()
		if time.Now().Before(session.expires) {
			err := session.err
			session.mu.Unlock()
			return err
		}
		if pending := session.running; pending != nil {
			session.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pending:
				continue
			}
		}
		session.running = make(chan struct{})
		session.mu.Unlock()

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		payload := []byte("image object storage readiness")
		_, err := session.store.putImageObject(probeCtx, session.healthKey, "text/plain", payload)
		if err == nil {
			var content []byte
			content, err = session.store.fetchImageObjectBytes(probeCtx, session.healthKey)
			if err == nil && !bytes.Equal(content, payload) {
				err = errors.New("object storage readiness content mismatch")
			}
		}
		cancel()
		if err != nil {
			err = errors.New("image object storage is unavailable")
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		session.mu.Lock()
		session.err = err
		ttl := 10 * time.Second
		if err != nil {
			ttl = 2 * time.Second
		}
		session.expires = time.Now().Add(ttl)
		if ctx.Err() != nil {
			session.expires = time.Time{}
		}
		close(session.running)
		session.running = nil
		session.mu.Unlock()
		return err
	}
}
