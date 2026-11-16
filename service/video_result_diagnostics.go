package service

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

var errVideoResultSourceRead = errors.New("video result source read failed")

// A source read can fail while the store is streaming the upload.
type videoResultSourceReader struct{ io.Reader }

func (r videoResultSourceReader) Read(buffer []byte) (int, error) {
	n, err := r.Reader.Read(buffer)
	if err != nil && err != io.EOF {
		return n, errors.Join(errVideoResultSourceRead, err)
	}
	return n, err
}

const (
	videoDeliveryStateStage   = "video_delivery_state"
	videoDeliverySourceStage  = "video_delivery_source"
	videoDeliveryStorageStage = "video_delivery_storage"
)

func newVideoDeliveryFailure(row *model.TaskVideoResultObject, task *model.Task, stage, reason string, now int64) *model.VideoDeliveryError {
	event := &model.VideoDeliveryError{
		CreatedAt: now, NextAttemptAt: now,
		TaskID: clienterrlog.SanitizeLogValue(row.TaskID, 64), Part: row.Part, UserID: row.UserID,
		Stage: stage, Reason: reason,
	}
	if task != nil {
		event.Model = clienterrlog.SanitizeLogValue(task.Properties.OriginModelName, 128)
		event.ChannelID = task.ChannelId
		if task.PrivateData.Execution != nil {
			event.RequestID = clienterrlog.SanitizeLogValue(task.PrivateData.Execution.RequestID, 64)
		}
	}
	return event
}

func runVideoDeliveryDiagnostics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var lastHealthCheck time.Time
	for now := time.Now(); ; now = <-ticker.C {
		if now.Sub(lastHealthCheck) >= time.Minute {
			lastHealthCheck = now
			checkVideoResultStorageHealth(now)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		if err := replayVideoDeliveryErrors(ctx, now.Unix()); err != nil {
			logger.LogWarn(ctx, "video delivery error log replay incomplete")
		}
		cancel()
	}
}

func replayVideoDeliveryErrors(ctx context.Context, now int64) error {
	pending, err := model.PendingVideoDeliveryErrors(ctx, now, 16)
	if err != nil {
		return err
	}
	var replayErr error
	for _, event := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := model.DeliverVideoDeliveryError(ctx, event, now); err != nil {
			replayErr = errors.Join(replayErr, err)
		}
	}
	return replayErr
}
