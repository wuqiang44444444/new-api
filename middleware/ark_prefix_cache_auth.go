package middleware

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

const arkCachePilotAuthKey = service.ArkCacheBindingKey
const arkCacheResponsesAuthKey = "ark_cache_responses_auth"

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
		c.Set(arkCacheResponsesAuthKey, true)
		if pilot != nil {
			c.Set(arkCachePilotAuthKey, pilot)
		}
		if key == "" || len(key) > 191 {
			native(c)
			return
		}
		tokenAuthWithValidator(func(keyValue string) (*model.Token, error) {
			token, err := model.ValidateUserToken(keyValue)
			if !errors.Is(err, model.ErrTokenQuotaExhausted) || token == nil ||
				pilot != nil && !pilot.AllowsApplication(token.UserId, token.Id) {
				return token, err
			}
			route, ok := service.ReadArkCacheRequestRoute(c)
			if !ok {
				return token, err
			}
			var digest string
			if pilot != nil {
				digest = route.ReplayDigest(pilot, key)
			} else if setting.ArkDefaultSessionEnabled() && route.SessionWrite() && !route.Stream {
				digest = model.ArkSessionCacheKeyDigest(key)
			}
			if digest == "" {
				return token, err
			}
			prior, claimErr := model.FindArkPrefixCacheClaim(c.Request.Context(), token.UserId, token.Id, digest)
			if claimErr != nil || prior.Model != route.Model {
				return token, err
			}
			if pilot == nil {
				binding := service.ArkSessionBindingForClaim(c.Request.Context(), route.Model, prior)
				if binding == nil {
					return token, err
				}
				c.Set(arkCachePilotAuthKey, binding)
			}
			valid, readErr := model.ValidateUserTokenForTaskRead(keyValue)
			if readErr == nil {
				c.Set(service.ArkCacheReplayOnlyKey, digest)
			}
			return valid, readErr
		})(c)
	}
}

// Only the Ark Responses authenticator installs this request-scoped policy.
// Returning true still runs the normal Key group checks in applyTokenRoutingScope.
func authorizePaidArkSessionWithoutContract(c *gin.Context, token *model.Token) bool {
	if token.ContractId != 0 || !c.GetBool(arkCacheResponsesAuthKey) {
		return false
	}
	pilot, _ := c.Get(arkCachePilotAuthKey)
	binding, _ := pilot.(*service.ArkPrefixCachePilot)
	route, ok := service.ReadArkCacheRequestRoute(c)
	if !ok || !route.SessionWrite() ||
		(token.ModelLimitsEnabled && !TokenModelLimitAllows(token.GetModelLimitsMap(), route.Model)) {
		return false
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if token.Group != "" {
		group = token.Group
	}
	if binding == nil {
		binding = service.DefaultArkSessionBinding(c, route, token.UserId, token.Id, group)
		if binding == nil {
			// An unknown managed ID must reach the terminal 404 handler.
			if !setting.ArkDefaultSessionEnabled() || !strings.HasPrefix(route.Previous, model.ArkSessionCacheIDPrefix) {
				return false
			}
			c.Set(service.ArkCacheContractExemptKey, true)
			return true
		}
		c.Set(arkCachePilotAuthKey, binding)
	}
	if !binding.AllowsApplication(token.UserId, token.Id) || !route.PaidSession(binding) ||
		!service.PaidArkCacheRoute(c, binding, group) {
		return false
	}
	c.Set(service.ArkCacheContractExemptKey, true)
	return true
}

// Token setup resolves explicit channel pins after applying group scope.
func confirmPaidArkSessionContractExemption(c *gin.Context) bool {
	if !c.GetBool(service.ArkCacheContractExemptKey) {
		return false
	}
	pilot, _ := c.Get(arkCachePilotAuthKey)
	binding, ok := pilot.(*service.ArkPrefixCachePilot)
	if ok {
		return service.PaidArkCacheRoute(c, binding, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	}
	if !c.GetBool(arkCacheResponsesAuthKey) {
		return false
	}
	route, valid := service.ReadArkCacheRequestRoute(c)
	return valid && route.SessionWrite() && strings.HasPrefix(route.Previous, model.ArkSessionCacheIDPrefix)
}
