package relay

import (
	"io"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TryHandlePersistedImageTaskResponse(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) bool {
	if c == nil || info == nil || response == nil ||
		response.StatusCode != http.StatusAccepted ||
		!info.BillingTransferredToTask ||
		info.PersistedImageTask == nil {
		return false
	}
	if response.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		_ = response.Body.Close()
	}
	taskID := info.PersistedImageTask.ID
	location := "/v1/images/tasks/" + url.PathEscape(taskID)
	c.Header("Location", location)
	c.Header("Retry-After", "2")
	c.Header("X-Task-ID", taskID)
	c.JSON(http.StatusAccepted, info.PersistedImageTask)
	return true
}

func markSynchronousImageIdempotencyComplete(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil || info.TaskRelayInfo == nil ||
		info.TaskRelayInfo.ClientProtocol != model.TaskClientProtocolOpenAIImages {
		return
	}
	common.SetContextKey(c, constant.ContextKeyTaskIdempotencyRelease, true)
}
