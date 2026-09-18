package middleware

import (
	"errors"
	"github.com/QuantumNous/new-api/model"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaykittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
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

// Contract state is read after identity checks, before the saved Key route group.
func customerContractAuthGate(c *gin.Context, token *model.Token) (bool, error) {
	if c.Request != nil && contractHistoricalOperation(c) {
		return false, nil
	}
	version, _ := common.GetContextKeyType[int64](c, constant.ContextKeyAuthVersion)
	snapshot, err := service.CustomerContractForRequest(c, token.UserId, version, token.ContractId)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "合同授权暂不可用 / Contract authorization is unavailable")
	}
	return snapshot != nil, err
}
