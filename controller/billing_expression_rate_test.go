package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionExchangeRateExpressionValidation(t *testing.T) {
	for _, tc := range []struct {
		name, expression, rate string
		valid                  bool
	}{
		{"direct call", `tier("base", p * 7 / usd_exchange_rate())`, "7", true},
		{"alias hidden from smoke vectors", `let fx = usd_exchange_rate; tier("base", p > 2000000 ? p * 7 / fx() : p)`, "7", false},
		{"missing setting in untaken branch", `true ? tier("base", 1) : tier("other", 7 / usd_exchange_rate())`, "invalid", false},
		{"rate-free without setting", `tier("base", p * 2)`, "invalid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupBillingAliasOptionDB(t)
			saved := map[string]string{}
			require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
			prior := operation_setting.USDExchangeRate
			t.Cleanup(func() {
				require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
				require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(prior)))
			})
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{}`}))
			if tc.rate == "invalid" {
				require.Error(t, operation_setting.SetUSDExchangeRate(tc.rate))
			} else {
				require.NoError(t, operation_setting.SetUSDExchangeRate(tc.rate))
			}
			expressions, err := common.Marshal(map[string]string{"generic-rate-save": tc.expression})
			require.NoError(t, err)
			body, err := common.Marshal(OptionUpdateRequest{Key: "billing_setting.billing_expr", Value: string(expressions)})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
			UpdateOption(ctx)
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, tc.valid, response.Success, response.Message)
			var options []model.Option
			require.NoError(t, model.DB.Where("key = ?", "billing_setting.billing_expr").Find(&options).Error)
			if tc.valid {
				require.Len(t, options, 1)
				assert.JSONEq(t, string(expressions), options[0].Value)
			} else {
				assert.Empty(t, options)
			}
		})
	}
}
