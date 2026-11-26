package helper

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ResponsesDelivery tracks downstream write failures independently from the
// upstream protocol outcome of a converted Responses stream.
type ResponsesDelivery struct {
	err error
}

// Send records the protocol outcome before delivering the event. A client
// disconnect stops delivery without turning collected usage into a relay error.
func (d *ResponsesDelivery) Send(c *gin.Context, info *relaycommon.RelayInfo, eventType string, payload dto.ResponsesStreamResponse) (bool, *types.NewAPIError) {
	payload.Type = eventType
	data, err := common.Marshal(payload)
	if err != nil {
		return false, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.ObserveConvertedResponsesOutcome(info, &payload)
	if err := ResponseChunkData(c, payload, string(data)); err != nil {
		if d.disconnect(c.Request.Context(), info.StreamStatus, err) {
			return false, nil
		}
		return false, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	return true, nil
}

func (d *ResponsesDelivery) disconnect(ctx context.Context, status *relaycommon.StreamStatus, err error) bool {
	if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) &&
		!errors.Is(err, syscall.EPIPE) && !errors.Is(err, syscall.ECONNRESET) {
		return false
	}
	d.err = err
	status.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
	return true
}

func (d *ResponsesDelivery) Disconnected(status *relaycommon.StreamStatus) bool {
	return d.err != nil || status.OutcomeSnapshot().EndReason == relaycommon.StreamEndReasonClientGone
}

// Finish runs after the scanner has joined. An upstream EOF may have been
// recorded while the downstream still had events left to receive.
func (d *ResponsesDelivery) Finish(status *relaycommon.StreamStatus) {
	if d.err != nil {
		status.MarkClientGoneOnDelivery(d.err)
	}
}
