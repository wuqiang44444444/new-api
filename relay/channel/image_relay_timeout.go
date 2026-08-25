package channel

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const (
	imageRelayRequestTimeout      = 10 * time.Minute
	imageRelayClientClosedStatus  = 499
	imageRelayRequestCanceledCode = types.ErrorCode("request_canceled")
)

// ImageRelayTimeoutContext bounds the complete image relay lifecycle. A
// positive global relay timeout may shorten, but never extend, this boundary.
func ImageRelayTimeoutContext(parent context.Context, startTime time.Time) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, imageRelayRemainingTimeout(startTime))
}

// ImageRelayHTTPClient preserves the shared transport while applying the
// remaining image relay deadline through response-body consumption.
func ImageRelayHTTPClient(client *http.Client, startTime time.Time) *http.Client {
	bounded := *client
	remaining := imageRelayRemainingTimeout(startTime)
	if bounded.Timeout <= 0 || remaining < bounded.Timeout {
		bounded.Timeout = remaining
	}
	return &bounded
}

// ImageRelayClientCanceledError keeps caller cancellation out of channel
// health decisions. The provider may still have accepted the request, so it
// also remains non-retryable and is not persisted as a relay error log.
func ImageRelayClientCanceledError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		fmt.Errorf("image request canceled by client: %w", context.Canceled),
		imageRelayRequestCanceledCode,
		imageRelayClientClosedStatus,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

func imageRelayRemainingTimeout(startTime time.Time) time.Duration {
	limit := imageRelayRequestTimeout
	if common.RelayTimeout > 0 {
		globalLimit := time.Duration(common.RelayTimeout) * time.Second
		if globalLimit < limit {
			limit = globalLimit
		}
	}
	if !startTime.IsZero() {
		if elapsed := time.Since(startTime); elapsed > 0 {
			limit -= elapsed
		}
	}
	if limit <= 0 {
		return time.Nanosecond
	}
	return limit
}
