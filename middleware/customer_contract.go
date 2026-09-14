package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaykittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// applyCustomerContractRequest is the single request-side discount resolver
// used by native distribution, the dedicated Seedance route and the task
// post-resolution path. The contract provides the discount only: it never
// sets or overrides UsingGroup/TokenGroup, never adds a channel pin, never
// filters candidates and never suppresses native retry. Keys without a
// contract binding return immediately without a contract-table read. An
// enabled contract that does not list the model returns a nil fact so the
// request keeps native pricing; load, version and consistency anomalies fail
// closed.
func applyCustomerContractRequest(c *gin.Context, publicModel string) (*hosttypes.ContractBillingFact, error) {
	contractId, _ := common.GetContextKeyType[int](c, constant.ContextKeyTokenContractId)
	if contractId <= 0 {
		return nil, nil
	}
	if strings.TrimSpace(publicModel) == "" {
		return nil, fmt.Errorf("model is required by the contract")
	}
	authVersion, ok := common.GetContextKeyType[int64](c, constant.ContextKeyAuthVersion)
	if !ok || authVersion <= 0 {
		return nil, fmt.Errorf("%w: authorization version is unavailable", service.ErrCustomerContractUnavailable)
	}
	fact, err := service.ResolveContractEntityRule(
		common.GetContextKeyInt(c, constant.ContextKeyUserId),
		authVersion,
		contractId,
		publicModel,
	)
	if err != nil {
		return nil, err
	}
	if fact != nil {
		common.SetContextKey(c, constant.ContextKeyContractFact, fact)
	}
	return fact, nil
}

// validateCustomerContractTokenModelLimit re-checks the token model limit for
// paths that resolve the fact after distribution (task remix), where the
// native distributor check cannot see the model yet. This is the only extra
// gate beyond native checks for contract keys.
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

// ApplyCustomerContractResolvedModel covers model calls whose public model is
// resolved from an existing task only after the distributor has run (for
// example, video remix). It resolves the discount fact and re-checks the
// token model limit; the locked origin channel keeps following native
// affinity rules and never needs to appear in contract details.
func ApplyCustomerContractResolvedModel(c *gin.Context, publicModel string) (*hosttypes.ContractBillingFact, error) {
	fact, err := applyCustomerContractRequest(c, publicModel)
	if err != nil || fact == nil {
		return fact, err
	}
	if err := validateCustomerContractTokenModelLimit(c, publicModel); err != nil {
		return nil, err
	}
	return fact, nil
}

// applyCustomerContractDistributeGate resolves the discount fact for a
// distributed request. Resolution failure aborts with 503 (fail closed —
// never native fallback); an unlisted model aborts nothing and keeps native
// behavior.
func applyCustomerContractDistributeGate(c *gin.Context, publicModel string, shouldSelectChannel bool) bool {
	if !shouldSelectChannel {
		return false
	}
	if _, err := applyCustomerContractRequest(c, publicModel); err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "Contract authorization is unavailable", relaykittypes.ErrorCodeModelNotFound)
		return true
	}
	return false
}
