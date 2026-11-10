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
	const maxBody = 32 << 10
	var body []byte
	disposition := relaycommon.TaskCreateOutcomeUnknown
	if response.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(response.Body, maxBody+1))
		if err == nil && len(body) <= maxBody {
			disposition = taskCreateHTTPDisposition(info, response.StatusCode, body)
		} else if len(body) > maxBody {
			body = body[:maxBody]
		}
	}
	relaycommon.SetTaskCreateDisposition(c, disposition)
	if disposition == relaycommon.TaskCreateOutcomeUnknown {
		markAmbiguousTaskCreate(c, info)
	}
	return taskUpstreamSubmissionError(parseTaskUpstreamHTTPError(response.StatusCode, body, info), info)
}
