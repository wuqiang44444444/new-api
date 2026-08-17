package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveCustomerContractPriceSettings(t *testing.T) {
	t.Helper()
	groupRatios := ratio_setting.GroupRatio2JSONString()
	modelPrices := ratio_setting.ModelPrice2JSONString()
	modelRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(modelRatios))
	})
}

func TestCustomerContractDiscountAppliesAfterNativeGroupRatioInPriceHelpers(t *testing.T) {
	preserveCustomerContractPriceSettings(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"contract-price":0.87}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"contract-fixed":0.0002,"contract-per-call":0.0002}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"contract-token":1}`))

	newContextAndInfo := func(modelName string, withContract bool) (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("group", "contract-price")
		info := &relaycommon.RelayInfo{
			OriginModelName: modelName, UserGroup: "default", UsingGroup: "contract-price",
		}
		if withContract {
			info.ContractBillingFact = &hosttypes.ContractBillingFact{
				UserId: 1, ContractVersion: 3, PublicModel: modelName,
				RouteGroup: "contract-price", RatioUnits: 80_000_000,
			}
		}
		return ctx, info
	}

	ctx, native := newContextAndInfo("contract-fixed", false)
	nativePrice, err := ModelPriceHelper(ctx, native, 0, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, 87, nativePrice.QuotaToPreConsume)

	ctx, contracted := newContextAndInfo("contract-fixed", true)
	contractPrice, err := ModelPriceHelper(ctx, contracted, 0, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, 69, contractPrice.QuotaToPreConsume, "100 × 0.87 × 0.8 is truncated once by the existing fixed-price pre-consume path")

	ctx, perCall := newContextAndInfo("contract-per-call", true)
	perCallPrice, err := ModelPriceHelperPerCall(ctx, perCall)
	require.NoError(t, err)
	assert.Equal(t, 69, perCallPrice.Quota)

	ctx, token := newContextAndInfo("contract-token", true)
	tokenPrice, err := ModelPriceHelper(ctx, token, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, common.QuotaFromFloat(1000*0.87*0.8), tokenPrice.QuotaToPreConsume)
}

func TestCustomerContractPriceHelperRejectsCorruptFrozenRatio(t *testing.T) {
	preserveCustomerContractPriceSettings(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"contract-corrupt":0.001}`))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "contract-corrupt", UserGroup: "default", UsingGroup: "default",
		ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: hosttypes.CustomerContractRatioScale + 1},
	}

	_, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
	require.Error(t, err)
}
