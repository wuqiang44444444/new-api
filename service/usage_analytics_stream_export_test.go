package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAnalyticsStreamOutcomesInAllExports(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Token{}, &model.BatchJob{}, &model.TaskBillingDelivery{}, &model.ProviderChannelBillingDiscount{}))
	period, err := model.ResolveUsageAnalyticsPeriod("week", "2026-09-16", 1789920000)
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{UserId: 42, TokenId: 991, ChannelId: 991, CreatedAt: period.StartTimestamp + 1, Type: model.LogTypeConsume, Quota: 100, ModelName: "chat", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"stream_status":{"status":"error","end_reason":"client_gone"}}`},
		{UserId: 42, TokenId: 991, ChannelId: 991, CreatedAt: period.StartTimestamp + 86401, Type: model.LogTypeConsume, Quota: 200, ModelName: "chat", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"stream_status":{"status":"error","end_reason":"eof","errors":["response_failed"]}}`},
	}).Error)
	filters := model.CustomerExportFilters{StartTimestamp: period.StartTimestamp, EndTimestamp: period.EndTimestamp, Timezone: model.UsageAnalyticsTimezone, Language: "en", QuotaPerUnit: 100, CurrencyRate: 1}
	filters.UsageDiscounts, err = model.FreezeUsageAnalyticsDiscounts(context.Background(), period)
	require.NoError(t, err)
	for _, view := range []string{"customer", "upstream", "customers"} {
		t.Run(view, func(t *testing.T) {
			rows, err := usageAnalyticsExportRows(context.Background(), &model.CustomerExportJob{JobID: "stream-outcomes", TargetUserId: 42}, view, period, filters, customerExportScopeColumns{QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"})
			require.NoError(t, err)
			require.NotEmpty(t, rows)
			grand := rows[len(rows)-1]
			require.Equal(t, "grand_total", grand[0])
			header := usageAnalyticsExportHeader("en")
			cells := make(map[string]string, len(header))
			for i, name := range header {
				cells[name] = grand[i]
			}
			assert.Equal(t, "2", cells["Calls"])
			assert.Equal(t, "0", cells["Success"])
			assert.Equal(t, "1", cells["Failure"])
			assert.Equal(t, "1", cells["Cancelled"])
			assert.Equal(t, "300", cells["Net quota"])
		})
	}
}
