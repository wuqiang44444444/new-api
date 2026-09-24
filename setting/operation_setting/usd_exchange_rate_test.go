package operation_setting

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUSDExchangeRateValidatesAndStores(t *testing.T) {
	prior := USDExchangeRate
	defer func() {
		if err := SetUSDExchangeRate(fmt.Sprintf("%v", prior)); err != nil {
			t.Fatalf("restore default rate: %v", err)
		}
	}()

	require.NoError(t, SetUSDExchangeRate("6.76"))
	assert.Equal(t, 6.76, USDExchangeRate)
	rate, err := CurrentUsdExchangeRateContext()
	require.NoError(t, err)
	assert.Equal(t, 6.76, rate.Rate)
	assert.Equal(t, "USDExchangeRate", rate.SourceKey)
	assert.NoError(t, rate.Validate())

	// 非法值：写入被拒绝，读取失败关闭，旧值既不覆盖也不回填默认。
	for _, bad := range []string{"abc", "0", "-1", "NaN", "Inf", ""} {
		err := SetUSDExchangeRate(bad)
		assert.Error(t, err, "value %q must be rejected", bad)
		_, err = CurrentUsdExchangeRateContext()
		assert.Error(t, err, "value %q must leave the reader fail-closed", bad)
	}
	assert.Equal(t, 6.76, USDExchangeRate, "invalid writes must not touch the legacy var")

	// 合法新值恢复可用。
	require.NoError(t, SetUSDExchangeRate("7"))
	rate, err = CurrentUsdExchangeRateContext()
	require.NoError(t, err)
	assert.Equal(t, 7.0, rate.Rate)
}

func TestValidateUSDExchangeRateDoesNotStore(t *testing.T) {
	prior := USDExchangeRate
	defer func() {
		if err := SetUSDExchangeRate(fmt.Sprintf("%v", prior)); err != nil {
			t.Fatalf("restore default rate: %v", err)
		}
	}()

	require.NoError(t, SetUSDExchangeRate("6.76"))
	assert.Error(t, ValidateUSDExchangeRate("0"))
	assert.Error(t, ValidateUSDExchangeRate("NaN"))
	rate, err := CurrentUsdExchangeRateContext()
	require.NoError(t, err)
	assert.Equal(t, 6.76, rate.Rate, "validation must not mutate stored state")
}
