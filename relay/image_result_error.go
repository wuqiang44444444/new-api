package relay

import (
	"errors"

	"github.com/QuantumNous/new-api/service"
)

// imageResultError contains only platform-defined diagnostics. Never retain a
// wrapped transport/storage error: those can contain credentials or signed URLs.
type imageResultError struct {
	Code               string
	DownloadHTTPStatus int
}

func (e *imageResultError) Error() string { return e.Code }

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
