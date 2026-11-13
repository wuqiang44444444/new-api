package relay

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
)

// imageResultError contains only platform-defined diagnostics. Never retain a
// wrapped transport/storage error: those can contain credentials or signed URLs.
type imageResultError struct {
	Code               string
	DownloadHTTPStatus int
}

func (e *imageResultError) Error() string { return e.Code }

func imageDownloadTransportError(err error) *imageResultError {
	if service.IsProtectedFetchPolicyError(err) {
		return &imageResultError{Code: "result_download_policy_rejected"}
	}
	return &imageResultError{Code: "result_download_transport_failed"}
}

func recordImageDeliveryFailure(ctx context.Context, err error) {
	code, status := "image_format_delivery_failed", 0
	var result *imageResultError
	switch {
	case errors.As(err, &result):
		code, status = result.Code, result.DownloadHTTPStatus
	case errors.Is(err, context.DeadlineExceeded):
		code = "image_format_delivery_timeout"
	case errors.Is(err, context.Canceled):
		code = "image_format_delivery_cancelled"
	}
	logger.LogWarn(ctx, "image result delivery failed: code=%s download_http_status=%d", code, status)
	service.RecordImageDeliveryError(ctx, "result_delivery", errors.New(code))
}

// imageResultFailure keeps delivery failures unknown, even when the image GET
// returns a 4xx. A failed download is not a rejected generation or refund proof.
func imageResultFailure(err error) service.ImageTaskExecution {
	result := service.ImageTaskExecution{Outcome: service.ImageTaskOutcomeUnknown, FailureCode: "result_read_failed"}
	var delivery *imageResultError
	if errors.As(err, &delivery) {
		result.FailureCode = delivery.Code
		result.DownloadHTTPStatus = delivery.DownloadHTTPStatus
	}
	return result
}
