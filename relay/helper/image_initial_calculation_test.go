package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Async admission requires the calculation record to carry the exact positive
// legacy Token reservation. A zero-token fixture would miss this regression.
func TestLegacyImagePositiveReservationHasInitialCalculation(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{}`, "quota_setting.pre_consume_multiplier": "2",
	}))
	prices, ratios, groups := ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(ratios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groups))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"legacy-image-calculation":2.5}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"image-calculation":1}`))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "legacy-image-calculation", UserGroup: "image-calculation", UsingGroup: "image-calculation",
		Request: &dto.ImageRequest{Model: "legacy-image-calculation", N: common.GetPointer(uint(1))},
	}
	price, err := ModelPriceHelper(c, info, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 500, price.QuotaToPreConsume, "100 input tokens × 2 reservation multiplier × 2.5 model ratio")
	require.NotNil(t, info.BillingCalculation)
	assert.Equal(t, 500, info.BillingCalculation.Quota, "the public task calculation must agree with the amount reserved at admission")
}
