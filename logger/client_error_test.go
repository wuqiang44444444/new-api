package logger

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type clientErrorWriterFunc func([]byte) (int, error)

func (f clientErrorWriterFunc) Write(p []byte) (int, error) { return f(p) }

func TestClientErrorOutputDoesNotHoldGlobalLockDuringIO(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	common.LogWriterMu.Lock()
	previous := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = clientErrorWriterFunc(func(p []byte) (int, error) { close(entered); <-release; return len(p), nil })
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		<-done
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previous
		common.LogWriterMu.Unlock()
	})
	go func() { defer close(done); _ = WriteClientErrorEvent(time.Now(), "safe-id", "safe-message") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer not reached")
	}
	unlocked := make(chan struct{})
	go func() { common.LogWriterMu.Lock(); common.LogWriterMu.Unlock(); close(unlocked) }()
	select {
	case <-unlocked:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked output retained global logger lock")
	}
}

func TestClientErrorOutputPreservesWarnFormatAndReturnsErrors(t *testing.T) {
	common.LogWriterMu.Lock()
	previous := gin.DefaultErrorWriter
	common.LogWriterMu.Unlock()
	t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = previous; common.LogWriterMu.Unlock() })
	failure := errors.New("test output failure")
	for _, tc := range []struct {
		name   string
		writer io.Writer
		err    error
	}{
		{"error", clientErrorWriterFunc(func([]byte) (int, error) { return 0, failure }), failure},
		{"short", clientErrorWriterFunc(func(p []byte) (int, error) { return len(p) - 1, nil }), io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.LogWriterMu.Lock()
			gin.DefaultErrorWriter = tc.writer
			common.LogWriterMu.Unlock()
			require.ErrorIs(t, WriteClientErrorEvent(time.Now(), "safe-id", "event=test"), tc.err)
		})
	}
	var buffer bytes.Buffer
	common.LogWriterMu.Lock()
	gin.DefaultErrorWriter = &buffer
	common.LogWriterMu.Unlock()
	require.NoError(t, WriteClientErrorEvent(time.Date(2026, 9, 16, 1, 2, 3, 0, time.UTC), "safe-id", "event=test"))
	assert.Equal(t, "[WARN] 2026/09/16 - 01:02:03 | safe-id | event=test \n", buffer.String())
}
