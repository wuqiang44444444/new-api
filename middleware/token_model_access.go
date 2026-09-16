package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/clienterrlog"
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
			abortTokenModelAccess(c, request.Model, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
			return
		}
		if _, permitted := allowed[ratio_setting.FormatMatchingModelName(request.Model)]; !permitted {
			abortTokenModelAccess(c, request.Model, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": request.Model}))
			return
		}
		c.Next()
	}
}

// TokenModelAccessFromQuery applies the same allow-list to model-keyed GET and
// DELETE asset operations. The endpoint remains responsible for requiring the
// model parameter.
func TokenModelAccessFromQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
			c.Next()
			return
		}
		modelName := strings.TrimSpace(c.Query("model"))
		if modelName == "" {
			c.Next()
			return
		}
		value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		allowed, typeOK := value.(map[string]bool)
		if !ok || !typeOK {
			abortTokenModelAccess(c, modelName, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
			return
		}
		if _, permitted := allowed[ratio_setting.FormatMatchingModelName(modelName)]; !permitted {
			abortTokenModelAccess(c, modelName, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": modelName}))
			return
		}
		c.Next()
	}
}

func abortTokenModelAccess(c *gin.Context, modelName, message string) {
	clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
		Model: modelName, Stage: "model_access", Reason: "token_model_forbidden", PublicCode: "token_model_forbidden",
	})
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
		"message": message,
		"type":    "asset_error",
		"code":    "token_model_forbidden",
	}})
}
