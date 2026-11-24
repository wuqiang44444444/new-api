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
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}, &model.ProviderChannelBillingConfig{}))
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
	discounted := strings.Replace(facts, `"group_ratio":1`, `"group_ratio":0.25`, 1)
	logs[0].Other = strings.TrimSuffix(discounted, "}") + `,"admin_info":{"statement_original_quota":5}}`

	logs = append(logs,
		model.Log{UserId: 43, ChannelId: 9672, Type: model.LogTypeRefund, CreatedAt: start + 2, Quota: 1000000, Other: facts},
		model.Log{UserId: 42, ChannelId: 9672, Type: model.LogTypeConsume, CreatedAt: start + 3, Quota: 5, ModelName: "unknown-money", Other: `{"model_ratio":1}`},
		model.Log{UserId: 42, ChannelId: 9672, Type: model.LogTypeConsume, CreatedAt: start + 4, Quota: 10, ModelName: "unknown-money", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1}`},
		model.Log{UserId: 42, ChannelId: 9673, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 999, ModelName: "outside-supplier", Other: facts},
		model.Log{UserId: 42, ChannelId: 9671, Type: model.LogTypeConsume, CreatedAt: end, Quota: 999, ModelName: "outside-period", Other: facts},
	)
	require.NoError(t, model.DB.Create(&logs).Error)
	publishBillingExportTestDay(t, start)
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
	first := reconciliationTestCells(t, upstreamSummaryColumns, records[1])
	second := reconciliationTestCells(t, upstreamSummaryColumns, records[2])
	incomplete := reconciliationTestCells(t, upstreamSummaryColumns, records[3])
	assert.Equal(t, "9671", first["channel_id"])
	assert.Equal(t, "' +SUM(A1:A2)", first["channel"])
	assert.Equal(t, "'-model", first["provider_model"])
	assert.Equal(t, "501", first["requests"])
	assert.Equal(t, exportCurrencyAmount("505", scope), first["original_amount"])
	assert.Equal(t, exportCurrencyAmount("1", scope), first["reference_amount"], "discount rounds once per evidence row")
	assert.Equal(t, "0.12345678", first["discount"])
	assert.Equal(t, "1", first["version"])
	assert.Equal(t, "Frozen supplier", first["name"])
	assert.Equal(t, exportCurrencyAmount("-1000000", scope), second["original_amount"])
	assert.NotContains(t, second["original_amount"], "'", "signed refund amount remains numeric")
	assert.Empty(t, incomplete["original_amount"], "unknown money must not become zero")
	assert.NotEmpty(t, incomplete["notes"])
	assert.Empty(t, incomplete["cache_read"], "missing cache evidence is not an observed zero")
	assert.Empty(t, incomplete["write_standard"])
	assert.Equal(t, "1", incomplete["gaps"], "only the unknown amount counts as pending")
	for _, row := range records[1:] {
		cells := reconciliationTestCells(t, upstreamSummaryColumns, row)
		assert.NotEqual(t, "9673", cells["channel_id"])
		assert.Equal(t, url, cells["url"])
	}
	visible, total, err := model.ListCustomerExportJobs(context.Background(), actor.Id, 1, 20)
	require.NoError(t, err)
	assert.Len(t, visible, 1)
	assert.EqualValues(t, 1, total)

	oldFilters := filters
	oldFilters.FieldVersion = dailyBillingExportFieldVersion - 1
	oldFilterJSON, err := common.Marshal(oldFilters)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.CustomerExportJob{}).Where("job_id = ?", job.JobID).Update("filters", string(oldFilterJSON)).Error)
	retry, err := ResubmitUpstreamExportJob(actor.Id, job.JobID)
	require.NoError(t, err)
	retryFilters, err := retry.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobTypeUpstreamSummary, retry.JobType)
	assert.Equal(t, filters.Upstream, retryFilters.Upstream)
	assert.Equal(t, filters.BillingReportVersion, retryFilters.BillingReportVersion)
	assert.Equal(t, dailyBillingExportFieldVersion, retryFilters.FieldVersion)
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

func TestUpstreamSummaryExportRequiresPublishedScopeAndHonorsBatchGate(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9675, Type: model.LogTypeConsume, CreatedAt: start + 1, ModelName: "first"},
		{UserId: 42, ChannelId: 9675, Type: model.LogTypeConsume, CreatedAt: start + 2, ModelName: "second"},
	}).Error)
	publishBillingExportTestDay(t, start)
	filters := model.CustomerExportFilters{StartTimestamp: start, EndTimestamp: start + 86400, Upstream: &model.UpstreamExportScope{ChannelIds: []int{9675}}}
	emitted := 0
	consume := func(int, string, model.ProviderURLChannelModelSummary) error { emitted++; return nil }
	require.Error(t, model.ScanUpstreamExportSummary(context.Background(), filters, model.BillingStatementReadPolicy{}, consume))
	ctx, _, err := model.ResolveBillingReport(context.Background(), start, start+86400-1, "")
	require.NoError(t, err)
	blocked := errors.New("queue cancelled")
	err = model.ScanUpstreamExportSummary(ctx, filters, model.BillingStatementReadPolicy{BeforeBatch: func(context.Context) error { return blocked }}, consume)
	assert.ErrorIs(t, err, blocked)
	assert.Zero(t, emitted)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	err = model.ScanUpstreamExportSummary(ctx, filters, model.BillingStatementReadPolicy{}, consume)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestChannelSummaryExportKeepsSubmittedNamesDiscountAndScope(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingConfig{}, &model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}))
	actor := model.User{Id: 9681, Username: "channel-summary-admin", AffCode: "channel-summary-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	url := "https://shared.example"
	require.NoError(t, model.DB.Create(&[]model.Channel{{Id: 9681, Name: "Original channel", BaseURL: &url}, {Id: 9682, Name: "Other channel", BaseURL: &url}}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	end := time.Unix(start, 0).In(billingExportTimezone()).AddDate(0, 1, 0).Unix()
	require.NoError(t, model.SaveProviderChannelBillingName(context.Background(), 9681, "Manual supplier", actor.Id))
	discount := model.ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: 9681, Discount: decimal.RequireFromString("0.8"), Reason: "manual"}
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 0, actor.Id))
	facts := `{"group_ratio":1,"contract_applicable":false,"model_ratio":1}`
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9681, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 100, ModelName: "selected", Other: facts},
		{UserId: 42, ChannelId: 9681, Type: model.LogTypeConsume, CreatedAt: start + 86400, Quota: 100, ModelName: "selected", Other: facts},
		{UserId: 42, ChannelId: 9682, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 100, ModelName: "outside", Other: facts},
	}).Error)
	publishBillingExportTestDay(t, start, start+86400)
	job, err := SubmitUpstreamExportJob(context.Background(), actor.Id, CustomerExportRequest{JobType: model.CustomerExportJobTypeUpstreamSummary, StartTimestamp: start, EndTimestamp: end, Language: "en"}, []int{9681}, "")
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Empty(t, filters.Upstream.URLKey)
	assert.Equal(t, "Manual supplier", filters.Upstream.Channels[9681].UpstreamName)
	require.NoError(t, model.SaveProviderChannelBillingName(context.Background(), 9681, "Changed supplier", actor.Id))
	discount.Discount = decimal.RequireFromString("0.5")
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 1, actor.Id, start+86400))
	require.NoError(t, model.DB.Delete(&model.Channel{}, 9681).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("channel-summary-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "channel-summary-test", nil)
	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status, completed.Error)
	artifact := completed.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[artifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2, "same URL never broadens the selected channel scope")
	record := reconciliationTestCells(t, upstreamSummaryColumns, records[1])
	assert.Equal(t, "9681", record["channel_id"])
	assert.Equal(t, "Manual supplier", record["name"])
	assert.Equal(t, "Original channel", record["channel"])
	assert.Equal(t, "0.8", record["discount"])
	assert.Equal(t, "1", record["version"])
	fresh, err := SubmitUpstreamExportJob(context.Background(), actor.Id, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeUpstreamSummary, StartTimestamp: start, EndTimestamp: end,
		Language: "en", ReportVersion: filters.BillingReportVersion,
	}, []int{9681}, "")
	require.NoError(t, err)
	assert.NotEqual(t, job.JobID, fresh.JobID, "a changed coefficient must not reuse the older file")
	claimed, err = model.ClaimNextQueuedCustomerExportJob("channel-summary-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "channel-summary-test", nil)
	fresh, err = model.GetCustomerExportJobForOwner(fresh.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, fresh.Status, fresh.Error)
	freshArtifact := fresh.DecodeArtifact()
	require.NotNil(t, freshArtifact)
	require.Len(t, freshArtifact.Files, 1)
	freshRecords, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[freshArtifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, freshRecords, 2)
	freshRecord := reconciliationTestCells(t, upstreamSummaryColumns, freshRecords[1])
	assert.Contains(t, freshRecord["discount"], "×0.8")
	assert.Contains(t, freshRecord["discount"], "×0.5")
	assert.Equal(t, "2", freshRecord["version"])
	amountScope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate}
	assert.Equal(t, exportCurrencyAmount("160", amountScope), record["reference_amount"])
	assert.Equal(t, exportCurrencyAmount("130", amountScope), freshRecord["reference_amount"])

	details, err := SubmitUpstreamExportJob(context.Background(), actor.Id, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeUpstreamDetails, StartTimestamp: start, EndTimestamp: end,
		Language: "en", ReportVersion: filters.BillingReportVersion,
	}, []int{9681}, "")
	require.NoError(t, err)
	claimed, err = model.ClaimNextQueuedCustomerExportJob("channel-details-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "channel-details-test", nil)
	details, err = model.GetCustomerExportJobForOwner(details.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, details.Status, details.Error)
	detailArtifact := details.DecodeArtifact()
	require.NotNil(t, detailArtifact)
	require.Len(t, detailArtifact.Files, 1)
	detailRecords, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[detailArtifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, detailRecords, 3)
	assert.Equal(t, "0.8", reconciliationTestCells(t, upstreamDetailColumns, detailRecords[1])["discount"])
	assert.Equal(t, "0.5", reconciliationTestCells(t, upstreamDetailColumns, detailRecords[2])["discount"])
	assert.Equal(t, exportCurrencyAmount("80", amountScope), reconciliationTestCells(t, upstreamDetailColumns, detailRecords[1])["reference_amount"])
	assert.Equal(t, exportCurrencyAmount("50", amountScope), reconciliationTestCells(t, upstreamDetailColumns, detailRecords[2])["reference_amount"])
}
