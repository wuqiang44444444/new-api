package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidExchangeRateWritesPreserveDatabaseAndActiveRate(t *testing.T) {
	for _, mode := range []string{"http", "bulk"} {
		t.Run(mode, func(t *testing.T) {
			setupBillingAliasOptionDB(t)
			prior := operation_setting.USDExchangeRate
			t.Cleanup(func() { require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(prior))) })
			require.NoError(t, model.UpdateOption("USDExchangeRate", "7"))
			for _, invalid := range []string{"0", "-1", "NaN", "+Inf", "not-a-number", ""} {
				if mode == "http" {
					body, err := common.Marshal(OptionUpdateRequest{Key: "USDExchangeRate", Value: invalid})
					require.NoError(t, err)
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
					UpdateOption(ctx)
					var response struct {
						Success bool `json:"success"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					assert.False(t, response.Success, invalid)
				} else {
					require.Error(t, model.UpdateOptionsBulk(map[string]string{
						"USDExchangeRate": invalid, "fx_test_marker": "must-not-persist",
					}))
					var count int64
					require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "fx_test_marker").Count(&count).Error)
					assert.Zero(t, count)
				}
				var option model.Option
				require.NoError(t, model.DB.Where("key = ?", "USDExchangeRate").First(&option).Error)
				assert.Equal(t, "7", option.Value, "rejected input must not overwrite durable configuration")
				common.OptionMapRWMutex.RLock()
				cached := common.OptionMap["USDExchangeRate"]
				common.OptionMapRWMutex.RUnlock()
				assert.Equal(t, "7", cached)
				rate, err := operation_setting.CurrentUsdExchangeRateContext()
				require.NoError(t, err, "a rejected write must not poison active billing")
				assert.Equal(t, 7.0, rate.Rate)
			}
			require.NoError(t, model.UpdateOptionsBulk(map[string]string{"USDExchangeRate": "10"}))
			rate, err := operation_setting.CurrentUsdExchangeRateContext()
			require.NoError(t, err)
			assert.Equal(t, 10.0, rate.Rate)
		})
	}
}
