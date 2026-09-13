package service

import (
	"context"
	"encoding/base64"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSeedanceSettlementStatementFacts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		tokens, quota int
		tier          string
	}{
		{"refund across tier", 120000, 600, "low"}, {"equal hold", 300000, 3000, "high"}, {"supplement", 400000, 4000, "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 8991, 10000)
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 8991).Update("request_count", 1).Error)
			require.NoError(t, model.DB.AutoMigrate(&model.ProviderBillingDiscount{}, &model.ProviderBillingAudit{}))
			expr := `u("tokens") < 200000 ? tier("low", u("tokens") * 5 / 1000000) : tier("high", u("tokens") * 10 / 1000000)`
			task := makeSeedanceUsageTask(t, expr, 300000, 3000)
			task.Properties.OriginModelName = "statement-video"
			task.PrivateData.BillingContext.OriginModelName = "statement-video"
			snap := task.PrivateData.AsyncBilling.TieredSnapshot
			snap.EstimatedTier = "high"
			snap.UsageFacts = map[string]any{"tokens": float64(300000)}
			snap.UsageUnits = map[string]string{"tokens": "token"}
			task.PrivateData.BillingContext.TieredSnapshot = snap
			require.NoError(t, model.DB.Save(task).Error)
			initial := model.NewLogOther()
			initial.SetPublic("task_id", task.TaskID)
			initial.SetPublic("task_billing_event", "create")
			initial.SetPublic("is_task", true)
			initial.SetPublic("group_ratio", 1.0)
			initial.SetPublic("usage_units", snap.UsageUnits)
			// The create row is an initial hold, never measured output.
			initial.SetPublic("expr_b64", base64.StdEncoding.EncodeToString([]byte(expr)))
			require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 8991, Type: model.LogTypeConsume, ModelName: "statement-video", CreatedAt: time.Now().Unix(), Quota: 3000, Other: initial.JSONString()}).Error)
			reportSeedanceUsage(t, task, tc.tokens, "usage.completion_tokens")
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, tc.tokens))
			require.True(t, settleTaskTieredSnapshot(context.Background(), reloadTask(t, task.ID), tc.tokens))
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Order("id").Find(&logs).Error)
			require.Len(t, logs, 2)
			assert.Equal(t, tc.tokens, logs[1].CompletionTokens)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[1].Other, &other))
			assert.Equal(t, tc.tier, other["matched_tier"])
			require.IsType(t, map[string]any{}, other["usage_facts"])
			assert.Equal(t, float64(tc.tokens), other["usage_facts"].(map[string]any)["tokens"])
			frozen := reloadTask(t, task.ID).PrivateData.AsyncBilling.TieredSnapshot
			assert.Equal(t, float64(300000), frozen.UsageFacts["tokens"])
			statement, err := model.GetBillingCustomerStatement(8991, 1, time.Now().Unix()+10, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, 1, statement.Summary.Requests)
			assert.EqualValues(t, tc.tokens, statement.Summary.OutputTokens)
			assert.EqualValues(t, tc.quota, statement.Summary.NetQuota)
			upstream, err := model.GetProviderBillingSummary(1, time.Now().Unix()+10, 1000, 0, "", "", 1)
			require.NoError(t, err)
			require.Len(t, upstream.Channels, 1)
			require.Len(t, upstream.Channels[0].Models, 1)
			assert.EqualValues(t, tc.tokens, upstream.Channels[0].Models[0].Usage.OutputTokens)
			assert.EqualValues(t, 1, upstream.Channels[0].Models[0].Usage.Requests)
			assert.Equal(t, 10000+3000-tc.quota, getUserQuota(t, 8991))
			var user model.User
			require.NoError(t, model.DB.First(&user, 8991).Error)
			assert.Equal(t, 1, user.RequestCount, "settlement must not count the submitted request again")
		})
	}
}

func TestSeedanceNonFiniteSettlementWithContractRetainsFunding(t *testing.T) {
	truncate(t)
	seedUser(t, 8991, 10000)
	task := makeSeedanceUsageTask(t, `tier("base", 1.0 / (u("tokens") - 100))`, 300000, 1500)
	task.PrivateData.BillingContext.ContractFact = contractBillingFact()
	require.NoError(t, model.DB.Save(task).Error)
	reportSeedanceUsage(t, task, 100, "usage.completion_tokens")
	require.NotPanics(t, func() {
		require.True(t, settleTaskTieredSnapshot(context.Background(), task, 100))
	})
	after := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateFailed, after.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1500, after.Quota)
	assert.Equal(t, 10000, getUserQuota(t, 8991))
	var log model.Log
	require.NoError(t, model.LOG_DB.First(&log).Error)
	assert.Contains(t, log.Other, "quota_saturation")
}
