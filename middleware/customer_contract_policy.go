package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type contractModelOperation uint8

const (
	contractModelCall contractModelOperation = iota + 1
	contractModelDiscovery
	contractPluginModelCall
	contractPluginRoutePending
)

const contractModelOperationKey = "contract_model_operation"
const contractModelRestrictedKey = "contract_model_restricted"

func ModelTokenAuth() gin.HandlerFunc {
	return ModelTokenAuthWith(TokenAuth())
}

// RelayV1TokenAuth protects the relay group's current and future routes. Only
// the published resource operations and legacy stubs use ordinary Key auth.
func RelayV1TokenAuth() gin.HandlerFunc {
	resourceAuth, modelAuth := TokenAuth(), ModelTokenAuth()
	return func(c *gin.Context) {
		switch c.Request.Method + " " + c.FullPath() {
		case "POST /v1/files", "GET /v1/files", "GET /v1/files/:id", "GET /v1/files/:id/content",
			"GET /v1/batches", "GET /v1/batches/:id", "POST /v1/batches/:id/cancel",
			"GET /v1/video/generations/:task_id",
			"POST /v1/images/variations", "DELETE /v1/files/:id",
			"POST /v1/fine-tunes", "GET /v1/fine-tunes", "GET /v1/fine-tunes/:id",
			"POST /v1/fine-tunes/:id/cancel", "GET /v1/fine-tunes/:id/events", "DELETE /v1/models/:model":
			resourceAuth(c)
		default:
			modelAuth(c)
		}
	}
}

// ModelTokenAuthWith applies model admission to a Key authenticator that uses
// the shared TokenAuth validation path.
func ModelTokenAuthWith(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(contractModelOperationKey, contractModelCall)
		auth(c)
	}
}

func ModelDiscoveryTokenAuth() gin.HandlerFunc {
	auth := TokenAuth()
	return func(c *gin.Context) {
		c.Set(contractModelOperationKey, contractModelDiscovery)
		auth(c)
	}
}

func JimengModelTokenAuth() gin.HandlerFunc {
	readAuth, modelAuth := TokenAuth(), ModelTokenAuth()
	return func(c *gin.Context) {
		if jimengHistoricalOperation(c) {
			readAuth(c)
			return
		}
		modelAuth(c)
	}
}

// Only routes whose candidates all declare submit can authorize model
// access here. The decoder classifies the rest before any model invocation.
func PluginModelTokenAuth() gin.HandlerFunc {
	auth := TokenAuth()
	return func(c *gin.Context) {
		pinned, _ := common.GetContextKeyType[pluginruntime.PinnedRoute](c, pluginruntime.ContextKeyPinnedRoute)
		operation := contractPluginModelCall
		if len(pinned.Candidates) == 0 {
			operation = contractPluginRoutePending
		}
		for _, candidate := range pinned.Candidates {
			if candidate.Route.Type != pluginruntime.RouteTypeSubmit {
				operation = contractPluginRoutePending
				break
			}
		}
		c.Set(contractModelOperationKey, operation)
		auth(c)
	}
}

func authorizeResolvedPluginModelCall(c *gin.Context) bool {
	operation, _ := c.Get(contractModelOperationKey)
	if operation != contractPluginRoutePending {
		return true
	}
	c.Set(contractModelOperationKey, contractPluginModelCall)
	token := &model.Token{
		UserId:     c.GetInt("id"),
		Group:      common.GetContextKeyString(c, constant.ContextKeyTokenGroup),
		ContractId: common.GetContextKeyInt(c, constant.ContextKeyTokenContractId),
	}
	return applyTokenRoutingScope(c, token, common.GetContextKeyString(c, constant.ContextKeyUserGroup)) &&
		!rejectRestrictedPluginModelCall(c)
}

func APIKeyModelAccessRestricted(c *gin.Context) bool {
	return c.GetBool(contractModelRestrictedKey)
}

func rejectRestrictedAPIKeyModelCall(c *gin.Context) bool {
	if !APIKeyModelAccessRestricted(c) {
		return false
	}
	operation, _ := c.Get(contractModelOperationKey)
	if operation != contractModelCall {
		return false
	}
	const status = http.StatusForbidden
	const code = constant.ErrorCodeContractRequired
	message := i18n.T(c, i18n.MsgAPIKeyContractRequired)
	if abortOfficialVideo(c, status, message) {
		service.RecordAPIKeyContractRequired(c, "")
		return true
	}
	publicCode := code
	path := c.FullPath()
	switch {
	case common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol) == model.TaskClientProtocolModelArkV3:
		abortModelArkVideo(c, status, code, message)
	case c.GetString(RouteTagKey) == "asset":
		c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
			"type": "asset_error", "code": code, "message": message, "request_id": c.GetString(common.RequestIdKey),
		}})
	case path == "/v1/batches":
		c.AbortWithStatusJSON(status, gin.H{"success": false, "error": gin.H{
			"type": "permission_error", "code": code, "message": message,
		}})
	case path == "/v1/messages":
		publicCode = ""
		c.AbortWithStatusJSON(status, gin.H{"type": "error", "error": types.ClaudeError{
			Type: "permission_error", Message: common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
		}})
	case strings.HasPrefix(path, "/mj/") || strings.HasPrefix(path, "/:mode/mj/"):
		publicCode = ""
		abortWithMidjourneyMessage(c, status, 4, message)
	default:
		abortWithOpenAiMessage(c, status, message, types.ErrorCode(code))
	}
	service.RecordAPIKeyContractRequired(c, publicCode)
	return true
}

func rejectRestrictedPluginModelCall(c *gin.Context) bool {
	if !APIKeyModelAccessRestricted(c) {
		return false
	}
	service.RecordAPIKeyContractRequired(c, "")
	c.Abort()
	RespondTaskPluginError(c, &dto.TaskError{
		Code: constant.ErrorCodeContractRequired, Message: i18n.T(c, i18n.MsgAPIKeyContractRequired), StatusCode: http.StatusForbidden,
	})
	return true
}
