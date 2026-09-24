package helper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ResponsesCompact invokes ModelPriceHelper again after receiving usage.
// Re-estimation and settlement must retain the first pre-consume's rate.
func TestModelPriceHelperReestimateKeepsFrozenExchangeRate(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	prior := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(prior)))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"rate-lifecycle":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"rate-lifecycle":"tier(\"base\", p * 7 / usd_exchange_rate())"}`,
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{OriginModelName: "rate-lifecycle", UserGroup: "default", UsingGroup: "default"}
	require.NoError(t, operation_setting.SetUSDExchangeRate("7"))
	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	frozen := *info.TieredBillingSnapshot.UsdExchangeRate
	require.NoError(t, operation_setting.SetUSDExchangeRate("10"))
	// Exercise restoration from the accepted snapshot even without cached input.
	info.BillingRequestInput = nil
	price, err := ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, frozen, *info.TieredBillingSnapshot.UsdExchangeRate)
	assert.Equal(t, common.QuotaRound(2000.0/1e6*common.QuotaPerUnit), price.QuotaToPreConsume)
	settled, err := billingexpr.ComputeTieredQuota(info.TieredBillingSnapshot, billingexpr.TokenParams{P: 2000})
	require.NoError(t, err)
	assert.Equal(t, price.QuotaToPreConsume, settled.ActualQuotaAfterGroup)
	fresh := &relaycommon.RelayInfo{OriginModelName: "rate-lifecycle", UserGroup: "default", UsingGroup: "default"}
	_, err = ModelPriceHelper(ctx, fresh, 2000, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.Equal(t, 10.0, fresh.TieredBillingSnapshot.UsdExchangeRate.Rate)
	// Corrupt accepted facts cannot be repaired from current settings or input.
	info.TieredBillingSnapshot.UsdExchangeRate = nil
	_, err = ModelPriceHelper(ctx, info, 2000, &types.TokenCountMeta{})
	require.Error(t, err)
}
