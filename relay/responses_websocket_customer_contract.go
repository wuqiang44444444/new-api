package relay

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func (s *responsesWSSession) authorizeContractModel(c *gin.Context, modelName, forwardedModel string) *types.NewAPIError {
	if service.ActiveCustomerContract(c) == nil {
		return checkResponsesWSModelAccess(c, modelName)
	}
	err := service.ErrCustomerContractScope
	if modelName == forwardedModel {
		_, err = service.ResolveCustomerContractRequest(c, modelName)
	}
	if err == nil && s.lockedChannelID != 0 {
		err = service.ValidateCustomerContractChannel(c, modelName, s.lockedChannelID)
		if err == nil && common.GetContextKeyString(c, constant.ContextKeyUsingGroup) != s.lockedGroup {
			err = service.ErrCustomerContractScope
		}
	}
	if err == nil {
		return nil
	}
	status := http.StatusServiceUnavailable
	if errors.Is(err, service.ErrCustomerContractScope) {
		status = http.StatusForbidden
	}
	return types.NewErrorWithStatusCode(errors.New("合同范围内无可用模型或渠道 / Contract model or channel is unavailable"), types.ErrorCodeModelNotFound, status, types.ErrOptionWithSkipRetry())
}
