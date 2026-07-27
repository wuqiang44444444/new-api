package middleware

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
)

const TaskCreateContractResponseKey = "task_create_contract_response"
const TaskCreateContractErrorKey = "task_create_contract_error"

type TaskCreateContractError struct {
	Status int
	Body   any
}

type taskCreateCaptureWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
	size   int
}

func (w *taskCreateCaptureWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *taskCreateCaptureWriter) WriteHeaderNow() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
}

func (w *taskCreateCaptureWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *taskCreateCaptureWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

func (w *taskCreateCaptureWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *taskCreateCaptureWriter) Size() int {
	return w.size
}

func (w *taskCreateCaptureWriter) Written() bool {
	return w.status != 0 || w.size > 0
}

func (w *taskCreateCaptureWriter) Flush() {}

func TaskCreateResponseContract() gin.HandlerFunc {
	return func(c *gin.Context) {
		original := c.Writer
		capture := &taskCreateCaptureWriter{ResponseWriter: original}
		c.Writer = capture
		c.Next()
		c.Writer = original

		if contractError, ok := c.Get(TaskCreateContractErrorKey); ok {
			if typed, typeOK := contractError.(TaskCreateContractError); typeOK {
				original.Header().Del("Content-Length")
				c.JSON(typed.Status, typed.Body)
				return
			}
		}
		if response, ok := c.Get(TaskCreateContractResponseKey); ok && capture.Status() < http.StatusBadRequest {
			original.Header().Del("Content-Length")
			c.JSON(http.StatusOK, response)
			return
		}
		original.WriteHeader(capture.Status())
		_, _ = original.Write(capture.body.Bytes())
	}
}
