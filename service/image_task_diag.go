package service

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const imageTaskDiagEventName = "image_task_execution_diag"

// imageTaskDiagSink reuses the clienterrlog non-blocking bounded-queue base
// with its own instance and independent drop counters. It defines the separate
// task-execution event stream and never enters the request-scoped 4xx
// statistics. Logging is observation only: a blocked or failing writer is
// absorbed by the drop counters and can never stall admission, execution or
// settlement.
var imageTaskDiagSink = clienterrlog.NewSink(context.Background(), 1024, func(e clienterrlog.Event) error {
	return logger.WriteClientErrorEvent(e.At, e.RequestID, e.Message)
})

// emitImageTaskExecutionDiag submits one sanitized observation when an image
// task commits a failure or first-unknown transition. Submission never waits
// for disk I/O and never changes the error classification or settlement.
func emitImageTaskExecutionDiag(task *model.Task, status model.TaskStatus, result ImageTaskExecution) {
	defer func() {
		if recover() != nil {
			return
		}
	}()
	if task == nil {
		return
	}
	data := task.PrivateData.ImageTask
	if data == nil {
		return
	}
	stage := result.FailureCode
	if stage == "" {
		stage = "unclassified"
	}
	trusted := "false"
	if status == model.TaskStatusFailure || status == model.TaskStatusExpired {
		trusted = "true"
	}
	message := "event=" + imageTaskDiagEventName +
		" task_id=" + clienterrlog.SanitizeLogValue(task.TaskID, 64) +
		" status=" + imageTaskDiagStatusToken(status) +
		" platform=" + clienterrlog.SanitizeLogValue(string(task.Platform), 32) +
		" channel_type=" + strconv.Itoa(data.ChannelType) +
		" channel_id=" + strconv.Itoa(task.ChannelId) +
		" operation=" + clienterrlog.SanitizeLogValue(data.Operation, 32) +
		" stage=" + clienterrlog.SanitizeLogValue(stage, 64)
	if result.UpstreamStatus > 0 {
		message += " upstream_status=" + strconv.Itoa(result.UpstreamStatus)
	}
	if result.DownloadHTTPStatus > 0 {
		message += " download_status=" + strconv.Itoa(result.DownloadHTTPStatus)
	}
	if result.ProviderRequestID != "" {
		message += " provider_request_id=" + clienterrlog.SanitizeLogValue(result.ProviderRequestID, 64)
	}
	if result.ViolationMarker {
		message += " violation_marker=true"
	}
	message += " trusted=" + trusted
	imageTaskDiagSink.Submit(clienterrlog.Event{At: time.Now(), RequestID: clienterrlog.SanitizeLogValue(task.TaskID, 64), Message: message})
}

// imageTaskDiagStatusToken maps terminal statuses onto our own whitelist
// tokens; other values degrade to a controlled token instead of raw input.
func imageTaskDiagStatusToken(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusFailure, model.TaskStatusExpired:
		return "failed"
	case model.TaskStatusReconciliationRequired:
		return "reconciliation_required"
	default:
		return "other"
	}
}

// ImageTaskDiagHealth exposes this queue separately from request-scoped 4xx logs.
func ImageTaskDiagHealth() clienterrlog.Health { return imageTaskDiagSink.Health() }
