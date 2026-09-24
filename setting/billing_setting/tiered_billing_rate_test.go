package billing_setting

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestSmokeTestExprWithExchangeRate(t *testing.T) {
	prior := operation_setting.USDExchangeRate
	defer func() {
		require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprintf("%v", prior)))
	}()
	require.NoError(t, operation_setting.SetUSDExchangeRate("6.76"))

	// 汇率表达式在合法设置下通过保存校验。
	require.NoError(t, SmokeTestExpr(`tier("base", 0.30 / usd_exchange_rate() * 1000000)`))

	// 设置非法时保存校验失败关闭，不允许存入无法履约的表达式。
	require.Error(t, operation_setting.SetUSDExchangeRate("bogus"))
	err := SmokeTestExpr(`tier("base", 0.30 / usd_exchange_rate() * 1000000)`)
	require.Error(t, err)

	// 无汇率依赖的表达式不受该设置影响。
	require.NoError(t, operation_setting.SetUSDExchangeRate("6.76"))
	require.NoError(t, SmokeTestExpr(`tier("base", p * 2.5 + c * 15)`))
}
