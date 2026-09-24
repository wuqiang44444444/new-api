package helper

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachFrozenExchangeRate(t *testing.T) {
	prior := operation_setting.USDExchangeRate
	defer func() {
		require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprintf("%v", prior)))
	}()
	require.NoError(t, operation_setting.SetUSDExchangeRate("6.76"))

	// 无汇率依赖的表达式不引入该事实。
	free := billingexpr.RequestInput{}
	require.NoError(t, AttachFrozenExchangeRate(`tier("base", p * 2.5)`, &free, nil))
	assert.Nil(t, free.ExchangeRate)

	// 依赖汇率时冻结当前设置。
	dependent := billingexpr.RequestInput{}
	require.NoError(t, AttachFrozenExchangeRate(`tier("base", 2.0 / usd_exchange_rate())`, &dependent, nil))
	require.NotNil(t, dependent.ExchangeRate)
	assert.Equal(t, 6.76, dependent.ExchangeRate.Rate)

	// 设置非法时资金预扣前失败关闭。
	require.Error(t, operation_setting.SetUSDExchangeRate("not-a-number"))
	broken := billingexpr.RequestInput{}
	err := AttachFrozenExchangeRate(`tier("base", 2.0 / usd_exchange_rate())`, &broken, nil)
	require.Error(t, err)
	assert.Nil(t, broken.ExchangeRate)
}

func TestAttachFrozenExchangeRateReusesAcceptedFact(t *testing.T) {
	prior := operation_setting.USDExchangeRate
	t.Cleanup(func() { require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(prior))) })
	const expression = `tier("base", p * 7 / usd_exchange_rate())`
	require.NoError(t, operation_setting.SetUSDExchangeRate("7"))
	input := billingexpr.RequestInput{}
	require.NoError(t, AttachFrozenExchangeRate(expression, &input, nil))
	frozen := *input.ExchangeRate
	require.NoError(t, operation_setting.SetUSDExchangeRate("10"))
	require.NoError(t, AttachFrozenExchangeRate(expression, &input, nil))
	assert.Equal(t, frozen, *input.ExchangeRate)
	snapshot := &billingexpr.BillingSnapshot{UsdExchangeRate: &frozen}
	input.ExchangeRate = &billingexpr.ExchangeRateContext{Rate: 10}
	require.NoError(t, AttachFrozenExchangeRate(expression, &input, snapshot))
	assert.Equal(t, frozen, *input.ExchangeRate)
	snapshot.UsdExchangeRate = nil
	require.Error(t, AttachFrozenExchangeRate(expression, &input, snapshot))
}
