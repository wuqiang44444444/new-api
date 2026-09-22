package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAnalyticsCustomerRefundReturnsToTaskFinishDay(t *testing.T) {
	for _, state := range []TaskBillingState{TaskBillingStateSettled, TaskBillingStateAwaitingUsage, ""} {
		for _, delivered := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/delivered=%t", state, delivered), func(t *testing.T) {
				db := setupUsageAnalyticsTestDB(t)
				period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
				require.NoError(t, err)
				task := Task{ID: 100, TaskID: "refunded-video", UserId: 7, ChannelId: 21, Platform: "video", Status: TaskStatusSuccess,
					FinishTime: period.StartTimestamp + 10, BillingState: state,
					Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 11},
					VideoRefund: VideoRefund{VideoRefundState: "refunded", VideoRefundCompletedAt: period.EndTimestamp + 10}}
				require.NoError(t, db.Create(&task).Error)
				require.NoError(t, db.Create(&User{Id: 7, Username: "refund-customer"}).Error)
				original := Log{UserId: 7, ChannelId: 21, RequestId: "task-billing:100:create", Type: LogTypeConsume, Quota: 100,
					Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`}
				if state == "" {
					original.RequestId = "native-video-request"
					original.CreatedAt = period.StartTimestamp + 1
					original.Other = `{"is_task":true,"task_id":"refunded-video","contract_applicable":false,"group_ratio":1,"model_ratio":1}`
				}
				require.NoError(t, db.Create(&original).Error)

				event := TaskBillingDelivery{TaskRowID: 100, Event: "customer_refund", BeforeQuota: 100, CreatedAt: period.EndTimestamp + 10}
				if delivered {
					event.DeliveredAt = event.CreatedAt
					require.NoError(t, db.Create(&Log{UserId: 7, ChannelId: 21, RequestId: "task-billing:100:customer_refund", Type: LogTypeRefund, Quota: 100, CreatedAt: event.CreatedAt, Other: `{"task_billing_event":"customer_refund","contract_applicable":false,"group_ratio":1,"model_ratio":1}`}).Error)
				}
				require.NoError(t, db.Create(&event).Error)
				view, err := GetUsageCustomerView(context.Background(), period, 7)
				require.NoError(t, err)
				assert.EqualValues(t, 1, view.Total.TotalCalls)
				assert.Zero(t, view.Total.NetQuota)
				assert.Zero(t, view.Total.RowsMoneyPending)
				if delivered {
					assert.EqualValues(t, 100, view.Total.GrossQuota)
					assert.EqualValues(t, 100, view.Total.RefundQuota)
					assert.Zero(t, view.Total.RowsMissingMoney)
				} else {
					assert.EqualValues(t, 1, view.Total.RowsMissingMoney)
					assert.Zero(t, view.Total.RefundQuota)
				}
				overview, err := GetUsageCustomersOverview(context.Background(), period, "")
				require.NoError(t, err)
				assert.Equal(t, view.Total, overview.Total)
				upstream, err := GetUsageUpstreamView(context.Background(), period)
				require.NoError(t, err)
				assert.Equal(t, view.Total.RefundQuota, upstream.Total.RefundQuota)
				if delivered && state != TaskBillingStateAwaitingUsage {
					require.NotNil(t, upstream.Total.OriginalQuotaEstimate)
					assert.EqualValues(t, 100, *upstream.Total.OriginalQuotaEstimate, "customer refund is not a provider credit")
				}
				next, err := ResolveUsageAnalyticsPeriod("day", "2026-09-17", 0)
				require.NoError(t, err)
				later, err := GetUsageCustomerView(context.Background(), next, 7)
				require.NoError(t, err)
				assert.Zero(t, later.Total.TotalCalls)
				assert.Zero(t, later.Total.RefundQuota)
			})
		}
	}
}
