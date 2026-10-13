package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRefundLogProjectsCommittedEventAmounts(t *testing.T) {
	for _, eventName := range []string{"refund", "customer_refund"} {
		t.Run(eventName, func(t *testing.T) {
			task := &model.Task{TaskID: "task-refund", Quota: 999,
				PrivateData: model.TaskPrivateData{BillingSource: BillingSourceSubscription, AsyncBilling: &model.TaskAsyncBillingContext{}}}
			log, err := BuildTaskBillingDeliveryLog(task, model.TaskBillingDelivery{
				Event: eventName, BeforeQuota: 100, AfterQuota: 0,
			})
			require.NoError(t, err)
			require.NotNil(t, log)
			var amounts struct {
				Before *int   `json:"adjustment_before_quota"`
				After  *int   `json:"actual_quota"`
				Source string `json:"billing_source"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &amounts))
			require.NotNil(t, amounts.Before)
			require.NotNil(t, amounts.After)
			assert.Equal(t, 100, *amounts.Before)
			assert.Zero(t, *amounts.After)
			assert.Equal(t, BillingSourceSubscription, amounts.Source)
			assert.Equal(t, model.LogTypeRefund, log.Type)
			assert.Equal(t, 100, log.Quota)
			assert.Equal(t, 999, task.Quota, "projection must not mutate Task funding")
		})
	}
}

func TestLegacyRefundLogUsesAtomicRefundResult(t *testing.T) {
	truncate(t)
	seedUser(t, 9931, 900)
	seedChannel(t, 9931)
	seedChargedAccounting(t, 9931, 9931, 0, 100, 1)
	task := makeTask(9931, 9931, 100, 0, BillingSourceWallet, 0)
	task.TaskID = "task-legacy-cost"
	require.NoError(t, model.DB.Create(task).Error)
	require.True(t, RefundTaskQuota(context.Background(), task, "task failed"))
	log := getLastLog(t)
	require.NotNil(t, log)
	var amounts struct {
		Before *int `json:"adjustment_before_quota"`
		After  *int `json:"actual_quota"`
	}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &amounts))
	require.NotNil(t, amounts.Before)
	require.NotNil(t, amounts.After)
	assert.Equal(t, 100, *amounts.Before)
	assert.Zero(t, *amounts.After)
	assert.Equal(t, 100, log.Quota)
	assert.Equal(t, 1000, getUserQuota(t, 9931))
	assert.Zero(t, task.Quota)
}
