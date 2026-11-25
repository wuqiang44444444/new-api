package middleware

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ArkPrefixCacheTokenAuth preserves normal relay authorization. The sole quota
// exception is a replay of a durable claim owned by the configured pilot app.
func ArkPrefixCacheTokenAuth() gin.HandlerFunc {
	return arkPrefixCacheTokenAuth(service.ArkCachePilot)
}

func ArkCacheDeleteTokenAuth() gin.HandlerFunc {
	return tokenAuthWithValidator(model.ValidateUserTokenForTaskRead)
}

func arkPrefixCacheTokenAuth(pilotForRequest func() *service.ArkPrefixCachePilot) gin.HandlerFunc {
	native := TokenAuth()
	return func(c *gin.Context) {
		pilot := pilotForRequest()
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if pilot == nil || key == "" || len(key) > 191 {
			native(c)
			return
		}
		tokenAuthWithValidator(func(keyValue string) (*model.Token, error) {
			token, err := model.ValidateUserToken(keyValue)
			if !errors.Is(err, model.ErrTokenQuotaExhausted) || token == nil || token.Id != pilot.TokenID || token.UserId != pilot.UserID {
				return token, err
			}
			storage, bodyErr := common.GetBodyStorage(c)
			if bodyErr != nil {
				return token, err
			}
			fields, bodyErr := common.ReadBodyJSONFields(storage, "model", "caching.type", "caching.prefix", "previous_response_id")
			if bodyErr != nil || len(fields) != 4 || fields[0].String() != pilot.Model || fields[1].String() != "enabled" {
				return token, err
			}
			parent, prefix := fields[3].String(), fields[2].Bool()
			var digest string
			switch {
			case prefix && parent == "":
				digest = model.ArkPrefixCacheKeyDigest(key)
			case !prefix && (parent == "" || strings.HasPrefix(parent, model.ArkSessionCacheIDPrefix)):
				digest = model.ArkSessionCacheKeyDigest(key)
			default:
				return token, err
			}
			if _, claimErr := model.FindArkPrefixCacheClaim(c.Request.Context(), token.UserId, token.Id, digest); claimErr != nil {
				return token, err
			}
			return model.ValidateUserTokenForTaskRead(keyValue)
		})(c)
	}
}
