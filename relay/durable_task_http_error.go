package relay

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// Only a verified rejection permits releasing a durable provider POST hold.
func durableTaskHTTPFailure(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) *dto.TaskError {
	var body []byte
	if response.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(response.Body, 32<<10))
	}
	disposition := taskCreateHTTPDisposition(info, response.StatusCode, body)
	relaycommon.SetTaskCreateDisposition(c, disposition)
	if disposition == relaycommon.TaskCreateOutcomeUnknown {
		markAmbiguousTaskCreate(c, info)
	}
	return taskUpstreamSubmissionError(parseTaskUpstreamHTTPError(response.StatusCode, body, info), info)
}
