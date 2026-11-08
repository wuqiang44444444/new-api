package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskDeliveryUsesFrozenBillingMode(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		price      float64
		perCall    bool
		expression string
		quota      int
		violation  bool
	}{
		{name: "legacy token", mode: "token", price: -1, quota: 2988},
		{name: "legacy fixed", mode: "per_call", price: 0.01, perCall: true, quota: 5000},
		{name: "free fixed with reported tokens", mode: "per_call", perCall: true},
		{name: "token expression", mode: "token", expression: `p * 5 + c * 30`, quota: 2988},
		{name: "failed token task with violation fee", mode: "per_call", price: -1, quota: 200, violation: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 41, 10000)
			billing := &model.TaskBillingContext{ModelPrice: tc.price, GroupRatio: 1, PerCallBilling: tc.perCall}
			if tc.expression != "" {
				billing.TieredSnapshot = &billingexpr.BillingSnapshot{ExprString: tc.expression, GroupRatio: 1}
			}
			task := &model.Task{
				TaskID: "image-task", UserId: 41, ClientProtocol: model.TaskClientProtocolImageOpenAIV1,
				Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "customer-image"},
				PrivateData: model.TaskPrivateData{
					BillingContext: billing,
					ImageTask: &model.TaskImageExecutionData{
						ImageCount: 1, Usage: &dto.Usage{PromptTokens: 19, CompletionTokens: 196, TotalTokens: 215},
					},
				},
			}
			if tc.violation {
				task.Status = model.TaskStatusFailure
				task.PrivateData.ImageTask.Usage = nil
				task.PrivateData.ImageTask.ViolationMarker = true
				task.PrivateData.ImageTask.ViolationFeePolicy = &model.TaskImageViolationFeePolicy{Enabled: true, BaseAmount: 0.0004}
			}
			log, err := BuildTaskBillingDeliveryLog(task, model.TaskBillingDelivery{Event: "complete", BeforeQuota: 32, AfterQuota: tc.quota})
			require.NoError(t, err)
			log.CreatedAt = 1100
			require.NoError(t, model.LOG_DB.Create(log).Error)
			other, err := common.StrToMap(log.Other)
			require.NoError(t, err)
			admin, ok := other["admin_info"].(map[string]any)
			require.True(t, ok)
			snapshot, ok := admin["statement_snapshot"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.mode, snapshot["billing_mode"])

			statement, err := model.GetBillingCustomerStatement(context.Background(), 41, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			assert.Equal(t, tc.mode, statement.Groups[0].Models[0].BillingMode)
			assert.EqualValues(t, tc.quota, statement.Summary.NetQuota)
		})
	}
}

func TestModelArkTaskDeliveryKeepsMappingInAdminLogs(t *testing.T) {
	truncate(t)
	seedUser(t, 41, 10000)
	task := &model.Task{
		TaskID: "video-task", UserId: 41, ClientProtocol: model.TaskClientProtocolModelArkV3,
		Properties: model.Properties{OriginModelName: "customer-video", UpstreamModelName: "private-video"},
	}
	for _, event := range []model.TaskBillingDelivery{
		{Event: "create", AfterQuota: 500},
		{Event: "settle", BeforeQuota: 500, AfterQuota: 300},
	} {
		log, err := BuildTaskBillingDeliveryLog(task, event)
		require.NoError(t, err)
		other, err := common.StrToMap(log.Other)
		require.NoError(t, err)
		assert.NotContains(t, other, "upstream_model_name")
		assert.NotContains(t, other, "is_model_mapped")
		require.NoError(t, model.LOG_DB.Create(log).Error)

		model.FormatAdminLogs([]*model.Log{log})
		admin, err := common.StrToMap(log.Other)
		require.NoError(t, err)
		assert.Equal(t, "private-video", admin["upstream_model_name"])
		assert.Equal(t, true, admin["is_model_mapped"])
	}
	logs, total, err := model.GetUserLogs(41, model.LogFilter{}, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	for _, log := range logs {
		assert.Equal(t, "customer-video", log.ModelName)
		assert.NotContains(t, log.Other, "private-video")
	}
}
