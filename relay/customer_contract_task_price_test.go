package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractTaskPriceFreezesBaseAndRefreshesOnlyGroup(t *testing.T) {
	previous := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"first":0,"next":2}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previous)) })
	for _, expression := range []bool{false, true} {
		c, _ := gin.CreateTestContext(nil)
		info := &relaycommon.RelayInfo{UsingGroup: "first", ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: 50_000_000}, PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: 1}}
		info.PriceData.AddOtherRatio("seconds", 4)
		if expression {
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{EstimatedQuotaBeforeGroup: 4 * common.QuotaPerUnit, ExprString: "frozen-expression"}
		}
		require.NoError(t, refreshCustomerContractTaskPrice(c, info, "video"))
		assert.Zero(t, info.PriceData.Quota)
		price, restored := restoreCustomerContractTaskPrice(c, info)
		require.True(t, restored)
		info.PriceData = price
		common.SetContextKey(c, constant.ContextKeyAutoGroup, "next")
		require.NoError(t, refreshCustomerContractTaskPrice(c, info, "video"))
		assert.Equal(t, common.QuotaFromFloat(4*common.QuotaPerUnit), info.PriceData.Quota)
		assert.False(t, info.PriceData.FreeModel)
		if expression {
			assert.Equal(t, "frozen-expression", info.TieredBillingSnapshot.ExprString)
		}
	}
}

func TestCustomerContractRemixPreservesBusinessRatiosOnly(t *testing.T) {
	info := &relaycommon.RelayInfo{ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: 50_000_000}}
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{OriginTaskID: "origin"}
	info.PriceData.AddOtherRatio("seconds", 4)
	info.PriceData.GroupRatioInfo.GroupRatio = 99
	price := hosttypes.PriceData{GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 2}}
	mergeCustomerContractOriginRatios(info, &price)
	assert.Equal(t, 4.0, price.OtherRatios()["seconds"])
	assert.Equal(t, 2.0, price.GroupRatioInfo.GroupRatio)
}
