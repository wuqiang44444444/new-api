package service

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type apiEvidenceHTTPControlKey struct{}

// Preserve the real server writer for bounded post-rejection body capture;
// Gin's writer does not expose net/http's read deadline controller.
func APIRequestEvidenceHTTPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if evidenceEnabled() && mediaEvidenceRequestKind(r.Method, r.URL.Path) != "" {
			r = r.WithContext(context.WithValue(r.Context(), apiEvidenceHTTPControlKey{}, http.NewResponseController(w)))
		}
		next.ServeHTTP(w, r)
	})
}

func captureRejectedRequestRemainder(c *gin.Context, capture *ImageErrorEvidence, record *imageEvidenceExchange, writer *apiEvidenceWriter) {
	if writer.hijacked || c.Writer.Status() < 400 || record.EOF || record.Error != "" || c.Request.Body == nil {
		return
	}
	control, _ := c.Request.Context().Value(apiEvidenceHTTPControlKey{}).(*http.ResponseController)
	if control == nil || control.SetReadDeadline(time.Now().Add(100*time.Millisecond)) != nil {
		return
	}
	defer func() { _ = control.SetReadDeadline(time.Time{}) }()
	// Deliver the already decided rejection before waiting for any remaining input.
	c.Writer.Flush()
	_, _ = io.Copy(io.Discard, io.LimitReader(c.Request.Body, evidenceMaxBodyBytes()+1))
}
