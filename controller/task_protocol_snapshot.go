package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func attachTaskProtocolSnapshot(c *gin.Context, task *model.Task, info *relaycommon.RelayInfo) {
	if task == nil || info == nil || info.TaskRelayInfo == nil {
		return
	}
	task.ClientProtocol = info.TaskRelayInfo.ClientProtocol
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return
	}
	seconds := req.Seconds
	if seconds == "" && req.Duration > 0 {
		seconds = strconv.Itoa(req.Duration)
	}
	serviceTier, _ := req.Metadata["service_tier"].(string)
	task.PrivateData.ClientRequest = model.TaskClientRequestSnapshot{
		Prompt:             req.Prompt,
		Seconds:            seconds,
		Size:               req.Size,
		RemixedFromVideoID: info.OriginTaskID,
		ServiceTier:        serviceTier,
	}
}
