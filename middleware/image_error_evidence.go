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
		if service.HasAPIRequestEvidence(c.Request.Context()) {
			c.Next()
			return
		}
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
		defer finish()
		c.Next()
	}
}
