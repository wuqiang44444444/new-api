package controller

import (
	"errors"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Refresh only the selected group against already resolved model prices. Do
// not re-evaluate model configuration or expression request/time predicates.
func prepareCustomerContractRelayBilling(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) *types.NewAPIError {
	if info.ContractBillingFact == nil || info.TieredBillingSnapshot != nil {
		return nil
	}
	price := &info.PriceData
	base := float64(common.Max(promptTokens, common.PreConsumedQuota)+meta.MaxTokens) * price.ModelRatio
	if price.UsePrice {
		base = price.ApplyOtherRatiosToFloat(price.ModelPrice * common.QuotaPerUnit)
	}
	if math.IsNaN(base) || math.IsInf(base, 0) || base < 0 || math.IsNaN(price.GroupRatioInfo.GroupRatio) || math.IsInf(price.GroupRatioInfo.GroupRatio, 0) || price.GroupRatioInfo.GroupRatio < 0 {
		return types.NewErrorWithStatusCode(errors.New("invalid contract billing multiplier"), types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	amount, err := service.ApplyCustomerContractRatio(decimal.NewFromFloat(base).Mul(decimal.NewFromFloat(price.GroupRatioInfo.GroupRatio)), info.ContractBillingFact)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	quota, err := common.QuotaFromFloatStrict(amount.InexactFloat64())
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	price.QuotaToPreConsume = quota
	price.FreeModel = base*price.GroupRatioInfo.GroupRatio == 0 && !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && info.Billing == nil
	if info.Billing == nil {
		if price.FreeModel {
			return nil
		}
		return service.PreConsumeBilling(c, quota, info)
	}
	if err := info.Billing.Reserve(quota); err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	info.FinalPreConsumedQuota = info.Billing.GetPreConsumedQuota()
	return nil
}
