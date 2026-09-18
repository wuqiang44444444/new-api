package controller

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractPricingPreservesBatchScope(t *testing.T) {
	_, user, contract := setupCustomerContractControllerDB(t)
	previous, err := common.Marshal(billing_setting.GetBatchBillingExprCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": string(previous)}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{"default-model":"p * 7","batch-only":"p * 3"}`}))
	require.NoError(t, model.DB.Create(&model.Channel{Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Group: "default", Models: "default-model,batch-only"}).Error)
	for _, keyAuth := range []bool{false, true} {
		t.Run(fmt.Sprintf("key=%t", keyAuth), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", user.Id)
			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/pricing?execution_mode=batch&contract_id=%d", contract.Id), nil)
			if keyAuth {
				c.Set("token_id", 1)
				common.SetContextKey(c, constant.ContextKeyTokenContractId, contract.Id)
				common.SetContextKey(c, constant.ContextKeyAuthVersion, user.AuthVersion)
			}
			GetPricing(c)
			require.Equal(t, 200, recorder.Code)
			var response struct {
				ExecutionMode string `json:"execution_mode"`
				Data          []struct {
					Model    string             `json:"model_name"`
					Expr     string             `json:"billing_expr"`
					Discount string             `json:"contract_discount"`
					Ratios   map[string]float64 `json:"group_ratio"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, "batch", response.ExecutionMode)
			require.Empty(t, response.Data, "no Batch sources were selected by this contract")

		})
	}
}

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

func TestContractMixedOrdinaryAndBatchSourcesKeepDedicatedAdminPrices(t *testing.T) {
	admin, user, contract := setupCustomerContractControllerDB(t)
	previous, err := common.Marshal(billing_setting.GetBatchBillingExprCopy())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": string(previous)}))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"batch_billing_setting.batch_billing_expr": `{"default-model":"p * 7"}`}))
	batch := model.Channel{Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Group: "default", Models: "default-model"}
	require.NoError(t, model.DB.Create(&batch).Error)
	var ordinary model.Channel
	require.NoError(t, model.DB.Where("name = ?", "default-contract-channel").First(&ordinary).Error)
	snapshot, err := model.ReplaceCustomerContractEntity(model.ReplaceCustomerContractEntityParams{
		ContractId: contract.Id, AdminUserId: admin.Id, ExpectedVersion: contract.Version, Name: contract.Name, Reason: "mixed native source types",
		Rules: []model.CustomerContractEntityRuleInput{
			{PublicModel: "default-model", ChannelId: ordinary.Id, RouteGroup: "default", RatioUnits: 90_000_000},
			{PublicModel: "default-model", ChannelId: batch.Id, RouteGroup: "default", RatioUnits: 90_000_000},
		},
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Rules, 2)
	views, err := service.BuildContractEntityAdminViews([]model.ContractEntitySnapshot{*snapshot}, user.Group)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Len(t, views[0].Rules, 2)
	for _, rule := range views[0].Rules {
		assert.Equal(t, "0.9", rule.Discount)
		if rule.ChannelId == batch.Id {
			assert.Equal(t, "batch_expr", rule.Price.BillingMode)
			assert.Equal(t, "p * 7", rule.Price.BillingExpr)
		} else {
			assert.Equal(t, "per_token", rule.Price.BillingMode)
			assert.Empty(t, rule.Price.BillingExpr)
			assert.Equal(t, "0.9", rule.Price.FinalModelRatio)
		}
	}
	public, err := service.BuildContractEntityUserViews([]model.ContractEntitySnapshot{*snapshot})
	require.NoError(t, err)
	require.Len(t, public[0].Models, 1)
	assert.Equal(t, "0.9", public[0].Models[0].Discount)
}
