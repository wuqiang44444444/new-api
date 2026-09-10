package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchPricingRequiresExplicitScopeAndUsesDedicatedPrice(t *testing.T) {
	_, user, _ := setupCustomerContractControllerDB(t)
	previous, err := common.Marshal(billing_setting.GetBatchBillingExprCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": string(previous)}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{"contract-model":"p * 7"}`}))
	require.NoError(t, model.DB.Create(&model.Channel{Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Group: "default", Models: "contract-model", Key: "private-test-key"}).Error)
	for _, scope := range []string{"", "?execution_mode=batch"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("GET", "/api/pricing"+scope, nil)
		c.Set("id", user.Id)
		GetPricing(c)
		require.Equal(t, 200, recorder.Code)
		if scope != "" {
			assert.Contains(t, recorder.Body.String(), `"billing_expr":"p * 7"`)
			assert.Contains(t, recorder.Body.String(), `"execution_mode":"batch"`)
		} else {
			assert.NotContains(t, recorder.Body.String(), `p * 7`)
		}
		assert.NotContains(t, recorder.Body.String(), "private-test-key")
		assert.NotContains(t, recorder.Body.String(), "channel_id")
	}
}

func TestBatchContractModelsUseDedicatedPriceWithoutNativePrice(t *testing.T) {
	admin, user, _ := setupCustomerContractControllerDB(t)
	previous, err := common.Marshal(billing_setting.GetBatchBillingExprCopy())
	require.NoError(t, err)
	oldSelfUse := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = false
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = oldSelfUse
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": string(previous)}))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{"batch-only-contract":"p * 7"}`}))
	channel := model.Channel{Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Group: "default", Models: "batch-only-contract"}
	require.NoError(t, model.DB.Create(&channel).Error)
	contract, err := model.CreateCustomerContractEntity(model.CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "Batch", Enabled: true, Reason: "test", Rules: []model.CustomerContractEntityRuleInput{{PublicModel: "batch-only-contract", ChannelId: channel.Id, RouteGroup: "default", RatioUnits: 80000000}}})
	require.NoError(t, err)
	for _, configured := range []bool{true, false} {
		if !configured {
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{}`}))
		}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("GET", "/v1/models", nil)
		c.Set("id", user.Id)
		common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
		common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
		require.True(t, listCustomerContractModels(c, constant.ChannelTypeOpenAI))
		require.Equal(t, 200, recorder.Code)
		if configured {
			assert.Contains(t, recorder.Body.String(), `"id":"batch-only-contract"`)
		} else {
			assert.NotContains(t, recorder.Body.String(), `"id":"batch-only-contract"`)
		}
	}
}
