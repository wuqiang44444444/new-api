package service

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

type failingVideoWriter struct{ gin.ResponseWriter }

func (w failingVideoWriter) Write(b []byte) (int, error) { return 0, errors.New("closed") }
func TestVideoDeliveryWriterDoesNotEquateTruncationWithReceipt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	w := &videoDeliveryWriter{ResponseWriter: failingVideoWriter{c.Writer}}
	_, err := w.WriteString("task response")
	assert.Error(t, err)
	assert.True(t, w.failed)
}
