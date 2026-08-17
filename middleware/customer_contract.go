package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaykittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// applyCustomerContractRequest is the single request-side contract guard used
// by native distribution and the dedicated Seedance route. Native users return
// immediately without a contract-table read.
func applyCustomerContractRequest(c *gin.Context, publicModel string) (*hosttypes.ContractBillingFact, error) {
	if !common.GetContextKeyBool(c, constant.ContextKeyContractMode) {
		return nil, nil
	}
	if strings.TrimSpace(publicModel) == "" {
		return nil, fmt.Errorf("model is required by the customer contract")
	}
	version, ok := common.GetContextKeyType[int64](c, constant.ContextKeyContractVersion)
	if !ok {
		return nil, fmt.Errorf("customer contract version is unavailable")
	}
	fact, err := service.ResolveCustomerContractRule(
		common.GetContextKeyInt(c, constant.ContextKeyUserId),
		version,
		publicModel,
	)
	if err != nil {
		return nil, err
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, fact.RouteGroup)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, fact.RouteGroup)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, false)
	common.SetContextKey(c, constant.ContextKeyContractFact, fact)
	return fact, nil
}

func channelSatisfiesCustomerContract(channel *model.Channel, fact *hosttypes.ContractBillingFact) bool {
	if channel == nil || fact == nil || channel.Status != common.ChannelStatusEnabled {
		return false
	}
	if channel.Type == constant.ChannelTypeSeedanceLink {
		selected, err := model.GetEnabledSeedanceChannel(fact.RouteGroup, fact.PublicModel, channel.Id)
		return err == nil && selected != nil && selected.Id == channel.Id
	}
	return model.IsChannelEnabledForExactCustomerContractModel(fact.RouteGroup, fact.PublicModel, channel.Id)
}

func validateCustomerContractTokenModelLimit(c *gin.Context, publicModel string) error {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return nil
	}
	value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	limits, valid := value.(map[string]bool)
	matchingName := ratio_setting.FormatMatchingModelName(publicModel)
	if !exists || !valid || (!limits[publicModel] && !limits[matchingName]) {
		return fmt.Errorf("token model limit excludes customer contract model %q", publicModel)
	}
	return nil
}

func abortCustomerContractChannelUnavailable(c *gin.Context, fact *hosttypes.ContractBillingFact, detail string) {
	logger.LogError(c.Request.Context(), fmt.Sprintf(
		"customer contract channel unavailable: user=%d model=%q group=%q detail=%s",
		common.GetContextKeyInt(c, constant.ContextKeyUserId), fact.PublicModel, fact.RouteGroup, detail,
	))
	abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "The requested model does not exist", relaykittypes.ErrorCodeModelNotFound)
}

// ApplyCustomerContractResolvedModel covers model calls whose public model is
// resolved from an existing task only after the distributor has run (for
// example, video remix). It reuses the same exact-match guard and validates the
// locked origin channel without changing native requests.
func ApplyCustomerContractResolvedModel(c *gin.Context, publicModel string, lockedChannel *model.Channel) (*hosttypes.ContractBillingFact, error) {
	fact, err := applyCustomerContractRequest(c, publicModel)
	if err != nil || fact == nil {
		return fact, err
	}
	if err := validateCustomerContractTokenModelLimit(c, publicModel); err != nil {
		return nil, err
	}
	if lockedChannel != nil && !channelSatisfiesCustomerContract(lockedChannel, fact) {
		return nil, fmt.Errorf("locked channel is outside the customer contract")
	}
	return fact, nil
}
