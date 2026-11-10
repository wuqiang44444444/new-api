package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const videoContentTokenKey = "video_content_token"

// VideoContentTokenAuth defers the quota decision until the shared content
// handler has determined whether this is an owned ModelArk task.
func VideoContentTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenAuthWithValidator(func(key string) (*model.Token, error) {
			token, err := model.ValidateUserTokenForTaskRead(key)
			if err == nil {
				c.Set(videoContentTokenKey, token)
			}
			return token, err
		})(c)
	}
}

func RequireNativeVideoContentQuota(c *gin.Context) bool {
	value, _ := c.Get(videoContentTokenKey)
	token, _ := value.(*model.Token)
	if token == nil || token.ValidateRelayQuota() != nil {
		abortWithOpenAiMessage(c, http.StatusUnauthorized, common.TranslateMessage(c, i18n.MsgTokenInvalid))
		return false
	}
	return true
}
