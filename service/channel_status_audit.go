package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const channelStatusTestObservationKey = "channel_status_test_observation"

// PrepareChannelTestAudit gives the observation and its resulting transition the
// same request ID, including successful probes that restore a channel.
func PrepareChannelTestAudit(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(channelStatusTestObservationKey, true)
	if c.GetString(common.RequestIdKey) == "" {
		c.Set(common.RequestIdKey, common.NewRequestId())
	}
}

// ChannelStatusAuditForRequest freezes only safe metadata before async dispatch.
// Provider messages and codes are intentionally not copied into the audit.
func ChannelStatusAuditForRequest(c *gin.Context, apiError *types.NewAPIError) model.ChannelStatusAudit {
	observation := model.ChannelStatusAudit{}
	if c != nil {
		observation.RequestID = c.GetString(common.RequestIdKey)
		if c.GetBool(channelStatusTestObservationKey) {
			observation.Trigger = "channel_test_recovered"
			if apiError != nil {
				observation.Trigger = "channel_test_failed"
			}
		}
	}
	if apiError != nil {
		if observation.Trigger == "" {
			observation.Trigger = "relay_error"
		}
		if apiError.GetErrorCode() == types.ErrorCodeChannelResponseTimeExceeded {
			observation.Trigger = "response_time_exceeded"
		}
	}
	return observation
}
