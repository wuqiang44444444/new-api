package service

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSummaryExportQueuedFrozenAndIsolated(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	t.Cleanup(func() {
		model.DB.Where("channel_id IN ?", []int{9671, 9672}).Delete(&model.ProviderChannelBillingDiscount{})
	})
	actor := model.User{Id: 9671, Username: "summary-admin", AffCode: "summary-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	url := "https://summary.example"
	require.NoError(t, model.DB.Create(&[]model.Channel{{Id: 9671, Name: " +SUM(A1:A2)", BaseURL: &url}, {Id: 9672, Name: "Second", BaseURL: &url}, {Id: 9673, Name: "Outside"}}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	discount := model.ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: 9671, Discount: decimal.RequireFromString("0.12345678"), Reason: "supplier coefficient"}
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 0, actor.Id))
	require.NoError(t, model.SaveProviderURLGroupName(url, "Frozen supplier", actor.Id))
	facts := `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1,"cache_tokens":0,"cache_write_tokens":0,"upstream_model_name":"-model","statement_snapshot":{"billing_mode":"token"}}`
	logs := make([]model.Log, 501)
	for i := range logs {
		logs[i] = model.Log{UserId: 42, ChannelId: 9671, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 1, Other: facts}
	}
	logs = append(logs,
		model.Log{UserId: 43, ChannelId: 9672, Type: model.LogTypeRefund, CreatedAt: start + 2, Quota: 1000000, Other: facts},
		model.Log{UserId: 42, ChannelId: 9672, Type: model.LogTypeConsume, CreatedAt: start + 3, Quota: 5, ModelName: "unknown-money", Other: `{"model_ratio":1}`},
		model.Log{UserId: 42, ChannelId: 9672, Type: model.LogTypeConsume, CreatedAt: start + 4, Quota: 10, ModelName: "unknown-money", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1}`},
		model.Log{UserId: 42, ChannelId: 9673, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 999, ModelName: "outside-supplier", Other: facts},
		model.Log{UserId: 42, ChannelId: 9671, Type: model.LogTypeConsume, CreatedAt: end, Quota: 999, ModelName: "outside-period", Other: facts},
	)
	require.NoError(t, model.DB.Create(&logs).Error)
	request := CustomerExportRequest{JobType: model.CustomerExportJobTypeUpstreamSummary, StartTimestamp: start, EndTimestamp: end, Language: "en"}
	_, err := SubmitCustomerExportJob(actor.Id, 42, request)
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest)
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, nil, "")
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest)
	_, err = SubmitUpstreamExportJob(context.Background(), 42, request, []int{9671}, url)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	job, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9671, 9672}, url)
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobStatusQueued, job.Status)
	assert.Empty(t, store.uploaded, "submission must not scan and generate the file")
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, "Frozen supplier", filters.Upstream.GroupName)
	// Arrivals after submission must not shift the export's model membership
	// or increase the amount of an already exported model.
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9671, Type: model.LogTypeConsume, CreatedAt: start + 5, Quota: 999, Other: facts},
		{UserId: 42, ChannelId: 9672, Type: model.LogTypeConsume, CreatedAt: start + 5, Quota: 999, ModelName: "aaa-late", Other: facts},
	}).Error)
	// Current names, URL membership and coefficients cannot reinterpret an accepted export.
	discount.Discount = decimal.RequireFromString("0.9")
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 1, actor.Id))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 9671).Updates(map[string]interface{}{"name": "Changed", "base_url": "https://moved.example"}).Error)
	require.NoError(t, model.SaveProviderURLGroupName(url, "Changed supplier", actor.Id))
	claimed, err := model.ClaimNextQueuedCustomerExportJob("summary-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "summary-test", nil)
	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status, completed.Error)
	artifact := completed.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	assert.EqualValues(t, 3, artifact.LineCount, "one row per channel/model/mode, no parent totals")
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[artifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 4)
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate}
	assert.Equal(t, "9671", records[1][5])
	assert.Equal(t, "' +SUM(A1:A2)", records[1][6])
	assert.Equal(t, "'-model", records[1][7])
	assert.Equal(t, "501", records[1][14])
	assert.Equal(t, exportCurrencyAmount("501", scope), records[1][16])
	assert.Equal(t, exportCurrencyAmount("0", scope), records[1][17], "discount rounds once per evidence row")
	assert.Equal(t, "0.12345678", records[1][18])
	assert.Equal(t, "1", records[1][19])
	assert.Equal(t, "Frozen supplier", records[1][4])
	assert.Equal(t, "501", records[1][29])
	assert.Equal(t, exportCurrencyAmount("-1000000", scope), records[2][16])
	assert.NotContains(t, records[2][16], "'", "signed refund amount remains numeric")
	assert.Empty(t, records[3][16], "unknown money must not become zero")
	assert.NotEmpty(t, records[3][26])
	assert.Empty(t, records[3][11], "missing cache evidence is not an observed zero")
	assert.Empty(t, records[3][12])
	assert.Equal(t, exportCurrencyAmount("10", scope), records[3][27], "known subtotal survives incomplete money")
	for _, row := range records[1:] {
		assert.NotEqual(t, "9673", row[5])
		assert.Equal(t, url, row[3])
	}
	visible, total, err := model.ListCustomerExportJobs(context.Background(), actor.Id, 1, 20)
	require.NoError(t, err)
	assert.Len(t, visible, 1)
	assert.EqualValues(t, 1, total)
	retry, err := ResubmitUpstreamExportJob(actor.Id, job.JobID)
	require.NoError(t, err)
	retryFilters, err := retry.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobTypeUpstreamSummary, retry.JobType)
	assert.Equal(t, filters.Upstream, retryFilters.Upstream)
	_, err = model.CancelCustomerExportJob(retry.JobID, actor.Id)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actor.Id).Update("role", common.RoleCommonUser).Error)
	_, err = model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	visible, total, err = model.ListCustomerExportJobs(context.Background(), actor.Id, 1, 20)
	require.NoError(t, err)
	assert.Empty(t, visible)
	assert.Zero(t, total)
}

func TestUpstreamSummaryExportHonorsBatchGateAndGroupLimit(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9675, Type: model.LogTypeConsume, CreatedAt: 1, ModelName: "first"},
		{UserId: 42, ChannelId: 9675, Type: model.LogTypeConsume, CreatedAt: 1, ModelName: "second"},
	}).Error)
	filters := model.CustomerExportFilters{StartTimestamp: 1, EndTimestamp: 2, Upstream: &model.UpstreamExportScope{URLKey: "channel:9675", ChannelIds: []int{9675}}}
	emitted := 0
	consume := func(int, model.ProviderURLChannelModelSummary) error { emitted++; return nil }
	blocked := errors.New("queue cancelled")
	err := model.ScanUpstreamExportSummary(context.Background(), filters, model.BillingStatementReadPolicy{BeforeBatch: func(context.Context) error { return blocked }}, consume)
	assert.ErrorIs(t, err, blocked)
	assert.Zero(t, emitted)
	err = model.ScanUpstreamExportSummary(context.Background(), filters, model.BillingStatementReadPolicy{MaxGroups: 1}, consume)
	assert.ErrorContains(t, err, "group limit exceeded")
	assert.Zero(t, emitted, "never publish partial aggregation")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = model.ScanUpstreamExportSummary(ctx, filters, model.BillingStatementReadPolicy{}, consume)
	assert.ErrorIs(t, err, context.Canceled)
}
