package middleware

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relaykittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Resolve one frozen request scope before channel selection and billing.
func applyCustomerContractRequest(c *gin.Context, publicModel string) (*hosttypes.ContractBillingFact, error) {
	return service.ResolveCustomerContractRequest(c, publicModel)
}

// ApplyCustomerContractResolvedModel authorizes a new call locked to an origin task channel.
func ApplyCustomerContractResolvedModel(c *gin.Context, publicModel string) (*hosttypes.ContractBillingFact, error) {
	return service.ResolveCustomerContractLockedChannel(c, publicModel, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
}

func applyCustomerContractDistributeGate(c *gin.Context, publicModel string, shouldSelectChannel bool) bool {
	if !shouldSelectChannel {
		return false
	}
	if _, err := applyCustomerContractRequest(c, publicModel); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, service.ErrCustomerContractScope) {
			status = http.StatusForbidden
		}
		abortWithOpenAiMessage(c, status, "合同范围内无可用模型或渠道 / Contract model or channel is unavailable", relaykittypes.ErrorCodeModelNotFound)
		return true
	}
	return false
}

// True means the request does not use the saved Key's native routing scope.
// Historical resources use their own ownership checks, not current routing grants.
func customerContractAuthGate(c *gin.Context, token *model.Token) (bool, error) {
	operation, _ := c.Get(contractModelOperationKey)
	if operation == contractPluginRoutePending || c.Request != nil && operation == nil && contractHistoricalOperation(c) {
		return true, nil
	}
	version, _ := common.GetContextKeyType[int64](c, constant.ContextKeyAuthVersion)
	snapshot, err := service.CustomerContractForRequest(c, token.UserId, version, token.ContractId)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "合同授权暂不可用 / Contract authorization is unavailable")
		return false, err
	}
	if snapshot == nil && operation != nil && model.APIKeyContractRequired() {
		c.Set(contractModelRestrictedKey, true)
		if authorizePaidArkSessionWithoutContract(c, token) {
			return false, nil
		}
		return true, nil
	}
	return snapshot != nil, nil
}

func applyTokenRoutingScope(c *gin.Context, token *model.Token, userGroup string) bool {
	skipNativeGroup, err := customerContractAuthGate(c, token)
	if err != nil {
		return false
	}
	if token.Group != "" && !skipNativeGroup {
		if _, ok := service.GetUserUsableGroups(userGroup)[token.Group]; !ok {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgAuthTokenGroupForbidden, map[string]any{"Group": token.Group}))
			return false
		}
		if token.Group != "auto" && !ratio_setting.ContainsGroupRatio(token.Group) {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgAuthTokenGroupDeprecated, map[string]any{"Group": token.Group}))
			return false
		}
		userGroup = token.Group
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, userGroup)
	return true
}
