package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type videoDeliveryWriter struct {
	gin.ResponseWriter
	failed bool
}

func (w *videoDeliveryWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if err != nil || n != len(b) {
		w.failed = true
	}
	return n, err
}
func (w *videoDeliveryWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TrackVideoCreateDelivery records a write failure independently of optional
// body evidence. Successful server writes still do not prove client receipt.
func TrackVideoCreateDelivery(c *gin.Context, t *model.Task) func() {
	if !model.IsVideoFundTask(t) || t.ID == 0 {
		return func() {}
	}
	c.Set("video_delivery_task_row", t.ID)
	if c.GetBool("video_delivery_tracking") {
		return func() {}
	}
	return BeginVideoClientDelivery(c)
}

// Install outside response-contract buffering, so only writes to the real
// downstream writer count. The presenter supplies the persisted task identity.
func BeginVideoClientDelivery(c *gin.Context) func() {
	c.Set("video_delivery_tracking", true)
	original := c.Writer
	writer := &videoDeliveryWriter{ResponseWriter: original}
	c.Writer = writer
	return func() {
		defer func() {
			c.Writer = original
			c.Set("video_delivery_tracking", false)
		}()
		id := c.GetInt64("video_delivery_task_row")
		if id == 0 {
			return
		}
		state := "server_written"
		if writer.failed || c.Request.Context().Err() != nil {
			state = "write_failed"
		}
		if err := model.RecordVideoClientDelivery(id, state); err != nil {
			common.SysError("video client delivery observation could not be recorded")
		}
	}
}
