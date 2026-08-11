package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// TokenModelAccess applies the same token model allow-list used by the relay
// distributor to model-bearing platform API requests.
func TokenModelAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
			c.Next()
			return
		}

		var request struct {
			Model string `json:"model"`
		}
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			// Leave malformed request handling to the endpoint so its existing
			// validation envelope remains unchanged.
			c.Next()
			return
		}
		request.Model = strings.TrimSpace(request.Model)
		if request.Model == "" {
			c.Next()
			return
		}

		value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		allowed, typeOK := value.(map[string]bool)
		if !ok || !typeOK {
			abortTokenModelAccess(c, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
			return
		}
		if _, permitted := allowed[ratio_setting.FormatMatchingModelName(request.Model)]; !permitted {
			abortTokenModelAccess(c, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": request.Model}))
			return
		}
		c.Next()
	}
}

func abortTokenModelAccess(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
		"message": message,
		"type":    "asset_error",
		"code":    "token_model_forbidden",
	}})
}
