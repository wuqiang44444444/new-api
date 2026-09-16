package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ImageErrorEvidence observes only the published image endpoints. No changes
// to status, body, flush behavior or request validation are made by this hook.
func ImageErrorEvidence() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.URL.Path {
		case "/v1/images/generations", "/v1/images/edits", "/v1/edits", "/v1/images/variations":
		default:
			c.Next()
			return
		}
		capture := service.NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: c.GetString(common.RequestIdKey)})
		if capture == nil {
			c.Next()
			return
		}
		finish := service.CaptureImageClientResponse(c, capture)
		defer func() {
			capture.Index.UserID = c.GetInt("id")
			capture.Index.AppID = c.GetInt("token_id")
			capture.Index.TokenID = c.GetInt("token_id")
			capture.Index.ChannelID = c.GetInt("channel_id")
			finish()
		}()
		c.Next()
	}
}
