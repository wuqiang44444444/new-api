package relay

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"math"
)

const contractTaskPriceKey = "customer_contract_task_price"

type contractTaskPrice struct {
	price    hosttypes.PriceData
	snapshot *billingexpr.BillingSnapshot
}

func mergeCustomerContractOriginRatios(info *relaycommon.RelayInfo, price *hosttypes.PriceData) {
	if info.ContractBillingFact == nil || info.OriginTaskID == "" {
		return
	}
	for key, ratio := range info.PriceData.OtherRatios() {
		if !price.HasOtherRatio(key) {
			price.AddOtherRatio(key, ratio)
		}
	}
}

func customerContractTaskPriceFrozen(c *gin.Context) bool {
	_, ok := c.Get(contractTaskPriceKey)
	return ok
}

func restoreCustomerContractTaskPrice(c *gin.Context, info *relaycommon.RelayInfo) (hosttypes.PriceData, bool) {
	value, ok := c.Get(contractTaskPriceKey)
	if !ok {
		return hosttypes.PriceData{}, false
	}
	frozen := value.(contractTaskPrice)
	info.TieredBillingSnapshot = frozen.snapshot
	return frozen.price, true
}

func refreshCustomerContractTaskPrice(c *gin.Context, info *relaycommon.RelayInfo, modelName string) error {
	if info.ContractBillingFact == nil {
		return nil
	}
	if !customerContractTaskPriceFrozen(c) {
		price := info.PriceData
		price.ReplaceOtherRatios(info.PriceData.OtherRatios())
		c.Set(contractTaskPriceKey, contractTaskPrice{price: price, snapshot: info.TieredBillingSnapshot})
	}
	price := &info.PriceData
	price.GroupRatioInfo = helper.HandleGroupRatio(c, info)
	base := price.ModelRatio / 2 * common.QuotaPerUnit
	if price.UsePrice {
		base = price.ModelPrice * common.QuotaPerUnit
	}
	if info.TieredBillingSnapshot != nil {
		base = info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup
	}
	if math.IsNaN(base) || math.IsInf(base, 0) || base < 0 || math.IsNaN(price.GroupRatioInfo.GroupRatio) || math.IsInf(price.GroupRatioInfo.GroupRatio, 0) || price.GroupRatioInfo.GroupRatio < 0 {
		return errors.New("invalid contract billing multiplier")
	}
	amount, err := service.ApplyCustomerContractRatio(decimal.NewFromFloat(base).Mul(decimal.NewFromFloat(price.GroupRatioInfo.GroupRatio)), info.ContractBillingFact)
	if err != nil {
		return err
	}
	var quota int
	if info.TieredBillingSnapshot != nil {
		quota, err = billingexpr.QuotaRoundStrict(amount.InexactFloat64())
		info.TieredBillingSnapshot.GroupRatio = price.GroupRatioInfo.GroupRatio
		info.TieredBillingSnapshot.EstimatedQuotaAfterGroup = quota
	} else {
		quota, err = common.QuotaFromFloatStrict(amount.InexactFloat64())
		if err == nil && !common.StringsContains(constant.TaskPricePatches, modelName) {
			quota, err = common.QuotaFromFloatStrict(price.ApplyOtherRatiosToFloat(float64(quota)))
		}
	}
	if err != nil {
		return err
	}
	price.Quota, price.QuotaToPreConsume = quota, quota
	price.FreeModel = base*price.GroupRatioInfo.GroupRatio == 0 && info.Billing == nil && !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	return nil
}
