package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractInternalGroupPricingPreservesSpecialRatioAndMode(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "ordinary"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			_, user, contract := setupCustomerContractControllerDB(t)
			assert.False(t, service.IsUserSelectableGroup(user.Group, "contract-api"))
			previousRatio := ratio_setting.GroupGroupRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"contract-api":0.5}}`))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousRatio)) })
			path := "/api/pricing"
			if batch {
				previous, err := common.Marshal(billing_setting.GetBatchBillingExprCopy())
				require.NoError(t, err)
				t.Cleanup(func() {
					require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": string(previous)}))
				})
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{"contract-model":"p * 7"}`}))
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("type", constant.ChannelTypeAzureBatch).Error)
				require.NoError(t, model.DB.Where("channel_id = ?", contract.Rules[0].ChannelId).Delete(&model.Ability{}).Error)
				path += "?execution_mode=batch"
			}
			c, recorder := customerContractTokenContext(http.MethodGet, path, user, contract.Id)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"contract-model": true})
			GetPricing(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Data       []service.CustomerContractPricingView `json:"data"`
				Groups     map[string]string                     `json:"usable_group"`
				Ratios     map[string]float64                    `json:"group_ratio"`
				AutoGroups []string                              `json:"auto_groups"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Len(t, response.Data, 1)
			assert.Equal(t, "contract-model", response.Data[0].ModelName)
			assert.Equal(t, "0.8", response.Data[0].ContractDiscount)
			assert.Equal(t, map[string]float64{"contract-api": 0.4}, response.Data[0].GroupRatio)
			assert.Equal(t, []string{"contract-api"}, response.Data[0].EnableGroup)
			assert.Equal(t, map[string]float64{"contract-api": 0.5}, response.Ratios)
			assert.Contains(t, response.Groups, "contract-api")
			assert.Len(t, response.Groups, 1, "do not expose groups excluded by the contract or Key model limit")
			assert.Empty(t, response.AutoGroups)
			if batch {
				assert.Equal(t, "p * 7", response.Data[0].BillingExpr)
			}
			assert.False(t, service.IsUserSelectableGroup(user.Group, "contract-api"), "contract pricing must not expand native permissions")
		})
	}
}

func TestContractInternalGroupViewsKeepRealAvailability(t *testing.T) {
	admin, user, contract := setupCustomerContractControllerDB(t)
	c, recorder := customerContractAdminContext(http.MethodGet, "/api/user/1/contract", "", admin, user)
	GetCustomerContract(c)
	assert.NotContains(t, recorder.Body.String(), "group_allowed")
	assert.Contains(t, recorder.Body.String(), `"available":true`)

	for _, tc := range []struct {
		name          string
		channelStatus int
		enabled       bool
		availability  string
	}{
		{"internal route authorized", common.ChannelStatusEnabled, true, "available"},
		{"channel disabled", common.ChannelStatusManuallyDisabled, true, "unavailable"},
		{"contract disabled", common.ChannelStatusEnabled, false, "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", contract.Rules[0].ChannelId).Update("status", tc.channelStatus).Error)
			require.NoError(t, model.DB.Model(&model.CustomerContract{}).Where("id = ?", contract.Id).Update("enabled", tc.enabled).Error)
			c, recorder := customerContractTokenContext(http.MethodGet, "/api/user/self/contract", user, 0)
			GetSelfCustomerContract(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Data struct {
					Contracts []service.ContractEntityUserView `json:"contracts"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Len(t, response.Data.Contracts, 1)
			require.Len(t, response.Data.Contracts[0].Models, 2)
			row := response.Data.Contracts[0].Models[0]
			assert.Equal(t, "contract-model", row.Model)
			assert.Equal(t, "0.8", row.Discount)
			assert.Equal(t, tc.availability, row.Availability)
			assert.NotContains(t, recorder.Body.String(), "contract-api", "customer details must not expose internal groups")
		})
	}
}

func TestCustomerContractDiscoveryPreservesWildcardModelLimits(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	settings := model_setting.GetGeminiSettings()
	previous := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = true
	t.Cleanup(func() { settings.ThinkingAdapterEnabled = previous })
	const name = "gemini-2.5-flash-thinking-8192"
	require.NoError(t, model.DB.Model(&model.CustomerContractEntityRule{}).Where("contract_id = ? AND public_model = ?", contract.Id, "contract-model").Update("public_model", name).Error)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("model = ?", "contract-model").Update("model", name).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("models = ?", "contract-model").Update("models", name).Error)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gemini-2.5-flash-thinking-*":1}`))
	model.InitChannelCache()
	c, response := customerContractTokenContext(http.MethodGet, "/v1/models", user, contract.Id)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"gemini-2.5-flash-thinking-*": true})
	ListModels(c, constant.ChannelTypeOpenAI)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"id":"`+name+`"`)
}
