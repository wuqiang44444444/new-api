package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// 任务内容交付（结果代理）的证据记录。正文是经过平台的媒体字节，
// 一期只记录状态、字节数与取消/错误；媒体本身不入证据库。

// taskContentDeliveryRecorder 统计一次内容交付的实际写出字节。
type taskContentDeliveryRecorder struct {
	countingWriter *service.EvidenceCountingWriter
	taskID         string
}

func beginTaskContentDeliveryEvidence(c *gin.Context, task *model.Task) *taskContentDeliveryRecorder {
	if !service.IsTaskRequestEvidenceEnabled() {
		_ = task
		return nil
	}
	if task == nil {
		return nil
	}
	countingWriter := service.NewEvidenceCountingWriter(c.Writer)
	return &taskContentDeliveryRecorder{countingWriter: countingWriter, taskID: task.TaskID}
}

func (r *taskContentDeliveryRecorder) WriterFor(writer http.ResponseWriter) http.ResponseWriter {
	if r == nil {
		return writer
	}
	return r.countingWriter.Wrap(writer)
}

func (r *taskContentDeliveryRecorder) Done(statusCode int, err error) {
	if r == nil {
		return
	}
	service.CaptureTaskContentDeliveryEvidence(r.taskID, statusCode, r.countingWriter.Written(), err)
}
