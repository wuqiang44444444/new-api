package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIDUsesTaskCreationRequestInLiveAndPublishedDetails(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	url := "https://request-id.example/v1"
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "account", BaseURL: &url}).Error)
	require.NoError(t, db.Create(&Task{ID: 101, UserId: 7, ChannelId: 21, TaskID: "task-101",
		PrivateData: TaskPrivateData{Execution: &TaskExecutionSnapshot{RequestID: "gateway-create-request"}},
	}).Error)
	logs := []Log{
		{UserId: 7, ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 10, ModelName: "video", RequestId: "task-billing:101:create",
			Other: `{"is_task":true,"task_id":"task-101","task_billing_event":"create"}`},
		{UserId: 7, ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 11, ModelName: "video", RequestId: "task-billing:101:adjustment",
			Other: `{"is_task":true,"task_id":"task-101","task_billing_event":"adjustment"}`},
		{UserId: 8, ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 12, ModelName: "video", RequestId: "task-billing:101:refund",
			Other: `{"is_task":true,"task_id":"task-101","task_billing_event":"refund"}`},
		{UserId: 7, ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 13, ModelName: "image", RequestId: "task-billing:202:complete",
			Other: `{"is_task":true,"task_id":"task-202","task_billing_event":"create","admin_info":{"task_create_request_id":"gateway-image-request"}}`},
		{UserId: 7, ChannelId: 21, Type: LogTypeConsume, CreatedAt: start + 14, ModelName: "chat", RequestId: "gateway-chat-request"},
	}
	require.NoError(t, db.Create(&logs).Error)

	filter := UpstreamBillingDetailFilter{Start: start, End: end, ChannelIds: []int{21}}
	check := func(ctx context.Context) {
		details, err := GetUpstreamBillingDetails(ctx, filter, 1, 10, false)
		require.NoError(t, err)
		require.Len(t, details.Items, 5)
		assert.Equal(t, "gateway-create-request", details.Items[0].RequestId)
		assert.Equal(t, "gateway-create-request", details.Items[1].RequestId)
		assert.Empty(t, details.Items[2].RequestId, "another user's task cannot supply a request ID")
		assert.Equal(t, "gateway-image-request", details.Items[3].RequestId)
		assert.Equal(t, "gateway-chat-request", details.Items[4].RequestId)

		scoped := filter
		scoped.RequestId = "gateway-create-request"
		page, err := GetUpstreamBillingDetails(ctx, scoped, 2, 1, false)
		require.NoError(t, err)
		assert.EqualValues(t, 2, page.Total)
		require.Len(t, page.Items, 1)
		assert.EqualValues(t, logs[1].Id, page.Items[0].RowId)
		var exported []string
		require.NoError(t, ScanUpstreamBillingDetails(ctx, scoped, false, BillingStatementReadPolicy{}, func(item UpstreamBillingDetailItem) error {
			exported = append(exported, item.RequestId)
			return nil
		}))
		assert.Equal(t, []string{"gateway-create-request", "gateway-create-request"}, exported)

		scoped.RequestId = logs[0].RequestId
		internal, err := GetUpstreamBillingDetails(ctx, scoped, 1, 10, false)
		require.NoError(t, err)
		assert.Zero(t, internal.Total)
	}
	check(ctx)

	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	pinned, manifest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	require.NotEqual(t, "-", manifest.Version)
	filter.URLKey = url
	check(pinned)

	var legacy billingReportLine
	require.NoError(t, db.Where("generation = ? AND source_id = ?", manifest.Version, logs[0].Id).First(&legacy).Error)
	item, err := legacy.upstreamDetail(false)
	require.NoError(t, err)
	assert.Equal(t, "gateway-create-request", item.RequestId, "new reports freeze the gateway ID")
	item.RequestId = logs[0].RequestId
	raw, err := common.Marshal(item)
	require.NoError(t, err)
	require.NoError(t, db.Model(&legacy).Update("upstream", BillingStatementJSON(raw)).Error)
	check(pinned)

	require.NoError(t, db.Delete(&Task{}, 101).Error)
	details, err := GetUpstreamBillingDetails(pinned, filter, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 5)
	assert.Empty(t, details.Items[0].RequestId, "old report without surviving task evidence remains unknown")
	assert.Equal(t, "gateway-create-request", details.Items[1].RequestId, "new report retains its frozen request ID")
}
