package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Price absence must survive discovery, both editors, and reloading saved rules;
// it must neither hide the model nor grant/revoke source eligibility.
func TestCustomerContractCatalogAndSavedRulesKeepMissingPricesAbsent(t *testing.T) {
	for _, scenario := range []string{"missing model ratio", "explicit zero", "missing group ratio"} {
		t.Run(scenario, func(t *testing.T) {
			admin, user, _ := setupCustomerContractControllerDB(t)
			previousPrices := ratio_setting.ModelPrice2JSONString()
			previousSelfUse := operation_setting.SelfUseModeEnabled
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
				operation_setting.SelfUseModeEnabled = previousSelfUse
			})
			operation_setting.SelfUseModeEnabled = false
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
			switch scenario {
			case "missing model ratio":
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
			case "explicit zero":
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"contract-model":0,"default-model":0}`))
			case "missing group ratio":
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{}`))
			}
			model.InvalidatePricingCache()
			for _, handler := range []struct {
				name string
				run  func(*gin.Context)
			}{{"user catalog", GetCustomerContractCatalog}, {"template catalog", GetCustomerContractTemplateOptions}} {
				t.Run(handler.name, func(t *testing.T) {
					ctx, recorder := customerContractAdminContext(http.MethodGet, "/", "", admin, user)
					handler.run(ctx)
					require.Equal(t, http.StatusOK, recorder.Code)
					var response struct {
						Success bool                            `json:"success"`
						Data    customerContractCatalogResponse `json:"data"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					require.True(t, response.Success)
					require.Len(t, response.Data.Groups, 2)
					for _, group := range response.Data.Groups {
						require.Len(t, group.Models, 1)
						entry := group.Models[0]
						require.Len(t, entry.Sources, 1)
						assert.Equal(t, scenario != "missing group ratio", entry.Sources[0].Available)
						if scenario == "explicit zero" {
							require.NotNil(t, entry.Price)
							assert.Equal(t, "0", entry.Price.CurrentDiscountedPrice)
						} else {
							assert.Nil(t, entry.Price)
						}
						if scenario == "missing group ratio" {
							assert.Empty(t, group.NativeGroupRatio)
						}
					}
				})
			}
			ctx, recorder := customerContractAdminContext(http.MethodGet, "/", "", admin, user)
			GetCustomerContract(ctx)
			var saved struct {
				Success bool `json:"success"`
				Data    struct {
					Contracts []service.ContractEntityAdminView `json:"contracts"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &saved))
			require.True(t, saved.Success)
			require.Len(t, saved.Data.Contracts, 1)
			require.Len(t, saved.Data.Contracts[0].Rules, 2)
			for _, rule := range saved.Data.Contracts[0].Rules {
				if scenario == "explicit zero" {
					require.NotNil(t, rule.Price)
					assert.Equal(t, "0", rule.Price.CurrentDiscountedPrice)
				} else {
					assert.Nil(t, rule.Price)
				}
				if scenario == "missing group ratio" {
					assert.Empty(t, rule.NativeGroupRatio)
					assert.Empty(t, rule.EffectiveMultiplier)
				}
			}
		})
	}
}
