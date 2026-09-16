package logger

import (
	"fmt"
	"io"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// WriteClientErrorEvent uses the existing WARN output and rotation destination,
// but returns write errors to the isolated client-error worker. Unlike LogWarn,
// it is never called by a request goroutine. Existing logger semantics stay intact.
func WriteClientErrorEvent(at time.Time, requestID, message string) error {
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	common.LogWriterMu.RUnlock()
	// Rotation may close a captured file; report that failure without holding the
	// global logger lock across potentially blocked I/O.
	line := fmt.Sprintf("[WARN] %s | %s | %s \n", at.Format("2006/01/02 - 15:04:05"), requestID, message)
	n, err := io.WriteString(writer, line)
	if err == nil && n != len(line) {
		return io.ErrShortWrite
	}
	if err == nil {
		logCount.Add(1)
	} // Existing logger owns rotation; never invoke its fatal setup path here.
	return err
}
