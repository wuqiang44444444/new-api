package service

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamExportFrozenScopeAndCurrentAuthorization(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}, &model.ProviderChannelBillingConfig{}))
	actor := model.User{Id: 9601, Username: "upstream-export-admin", AffCode: "up-export", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	url := "https://upstream.example"
	require.NoError(t, model.DB.Create(&model.Channel{Id: 9601, Name: "=channel", BaseURL: &url}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	discount := model.ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: 9601, Discount: decimal.RequireFromString("0.12345678"), Reason: "confirmed supplier coefficient"}
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 0, actor.Id))
	t.Cleanup(func() { model.DB.Where("channel_id = ?", 9601).Delete(&model.ProviderChannelBillingDiscount{}) })
	const urlKey = "https://upstream.example"
	require.NoError(t, model.SaveProviderURLGroupName(urlKey, "Primary upstream", actor.Id))
	fallback := false
	request := CustomerExportRequest{StartTimestamp: start, EndTimestamp: end, RequestId: "export-match", ModelName: "same", ProviderModelFallback: &fallback, Language: "en"}
	_, err := SubmitUpstreamExportJob(context.Background(), 42, request, []int{9601}, "")
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	_, err = SubmitCustomerExportJob(actor.Id, 42, CustomerExportRequest{JobType: model.CustomerExportJobTypeUpstreamDetails, StartTimestamp: start, EndTimestamp: end})
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest, "single-customer endpoint must not admit the cross-customer type")
	facts := `{"contract_applicable":false,"group_ratio":0.5,"model_ratio":1,"upstream_model_name":"same","statement_snapshot":{"billing_mode":"per_second"},"usage_units":{"duration":"second"},"usage_facts":{"duration":6.5},"task_id":"task-public","upstream_task_id":"task-root"}`
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 500000, RequestId: "export-match", UpstreamRequestId: "up-1", Other: strings.TrimSuffix(facts, "}") + `,"admin_info":{"statement_original_quota":1000001}}`},
		{UserId: 43, ChannelId: 9601, Type: model.LogTypeRefund, CreatedAt: start + 2, Quota: 100000, RequestId: "export-match", UpstreamRequestId: "up-2", Other: strings.TrimSuffix(facts, "}") + `,"admin_info":{"statement_original_quota":200000}}`},
		{UserId: 43, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + 3, Quota: 999, RequestId: "excluded", Other: facts},
	}).Error)
	// Same-name fallback and a different known model must not leak into this file.
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + 4, Quota: 1, ModelName: "same", RequestId: "export-match", Other: `{"model_ratio":1,"is_model_mapped":true}`},
		{UserId: 42, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + 5, Quota: 1, ModelName: "other", RequestId: "export-match", Other: `{"model_ratio":1,"upstream_model_name":"other"}`},
	}).Error)
	// Cross both the UI page size and the reader's 500-row batch boundary.
	more := make([]model.Log, 500)
	for i := range more {
		more[i] = model.Log{UserId: 43, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + int64(i) + 10, Quota: 1, RequestId: "export-match", Other: facts}
	}
	more[0].Other = `{"contract_applicable":false,"group_ratio":1,"upstream_model_name":"same","statement_snapshot":{"billing_mode":"per_second"},"usage_units":{"duration":"second"}}`
	more[1].Other = `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"upstream_model_name":"same","request_path":"/v1/images/edits","cache_tokens":0,"cache_read_tokens_reported":false}`
	more[2].Other = `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1,"upstream_model_name":"same","request_path":"/v1/images/edits","cache_tokens":0,"cache_write_tokens":0,"cache_write_tokens_reported":true,"cache_read_tokens_reported":true,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":1}}`
	more[2].TokenName, more[2].Content = "模型测试", "模型测试"
	require.NoError(t, model.DB.Create(&more).Error)
	publishBillingExportTestDay(t, start)
	job, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9601}, urlKey)
	require.NoError(t, err)
	// Settings and coefficients changing after acceptance never alter the file.
	discount.Discount = decimal.RequireFromString("0.9")
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 1, actor.Id))
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9601}, urlKey)
	require.ErrorIs(t, err, model.ErrCustomerExportUserBusy, "an active job with an older coefficient cannot satisfy a new export")
	claimed, err := model.ClaimNextQueuedCustomerExportJob("upstream-test", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "upstream-test", nil)
	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status)
	artifact := completed.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	require.EqualValues(t, 502, artifact.LineCount)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[artifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 503)
	first := reconciliationTestCells(t, upstreamDetailColumns, records[1])
	second := reconciliationTestCells(t, upstreamDetailColumns, records[2])
	filters, err := completed.DecodeFilters()
	require.NoError(t, err)
	require.NotNil(t, filters.Upstream.ProviderModelFallback)
	assert.False(t, *filters.Upstream.ProviderModelFallback)
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate}
	assert.Equal(t, "'=channel", first["channel"])
	assert.Equal(t, exportCurrencyAmount("1000001", scope), first["original_amount"])
	assert.Equal(t, exportCurrencyAmount("-200000", scope), second["original_amount"])
	assert.Equal(t, "0.12345678", first["discount"])
	assert.Equal(t, "1", first["version"])
	assert.Equal(t, "up-1", first["upstream_request"])
	assert.Equal(t, "task-root", first["upstream_task"])
	assert.Equal(t, "退款", second["event"])
	assert.Equal(t, "6.5", first["seconds"])
	assert.Empty(t, second["seconds"], "customer refunds do not cancel provider seconds")
	assert.Equal(t, exportCurrencyAmount("123457", scope), first["reference_amount"])
	assert.Equal(t, exportCurrencyAmount("-24691", scope), second["reference_amount"])
	unknown := reconciliationTestCells(t, upstreamDetailColumns, records[3])
	assert.Empty(t, unknown["seconds"])
	assert.Equal(t, "已核算", unknown["status"])
	assert.Contains(t, unknown["notes"], "未保存当时的计费秒数")
	for _, i := range []int{4, 5} {
		row := reconciliationTestCells(t, upstreamDetailColumns, records[i])
		assert.Equal(t, "0", row["cache_read"])
		assert.Equal(t, "已核算", row["status"])
	}
	pricedTest := reconciliationTestCells(t, upstreamDetailColumns, records[5])
	assert.Equal(t, exportCurrencyAmount("1", scope), pricedTest["original_amount"])
	assert.Contains(t, pricedTest["notes"], "渠道测试计价 priced")
	referenceSum := decimal.Zero
	for _, record := range records[1:] {
		cell := reconciliationTestCells(t, upstreamDetailColumns, record)["reference_amount"]
		if cell == "" {
			continue
		}
		value, parseErr := decimal.NewFromString(cell)
		require.NoError(t, parseErr)
		referenceSum = referenceSum.Add(value)
	}
	assert.Equal(t, exportCurrencyAmount("98766", scope), referenceSum.StringFixed(8), "tiny rows round once before every aggregation")
	fresh, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9601}, urlKey)
	require.NoError(t, err)
	assert.NotEqual(t, completed.JobID, fresh.JobID)
	require.Equal(t, model.CustomerExportJobStatusQueued, fresh.Status)
	retry, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9601}, urlKey)
	require.NoError(t, err)
	assert.Equal(t, fresh.JobID, retry.JobID)
	_, err = model.CancelCustomerExportJob(fresh.JobID, actor.Id)
	require.NoError(t, err)

	// Regeneration keeps the frozen upstream name even after the alias changes.
	require.NoError(t, model.SaveProviderURLGroupName(urlKey, "Renamed upstream", actor.Id))
	resubmitted, err := ResubmitUpstreamExportJob(actor.Id, job.JobID)
	require.NoError(t, err)
	resubmitFilters, err := resubmitted.DecodeFilters()
	require.NoError(t, err)
	require.NotNil(t, resubmitFilters.Upstream)
	assert.Equal(t, "Primary upstream", resubmitFilters.Upstream.GroupName)
	// Release the per-user export slot so the later submission stays admitted;
	// the cancelled copy is root-only and stays invisible after demotion.
	_, err = model.CancelCustomerExportJob(resubmitted.JobID, actor.Id)
	require.NoError(t, err)
	_, err = model.GetCustomerExportJobForOwner(job.JobID, 43)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	// A root-only file cannot be fetched after demotion, even to administrator.
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actor.Id).Update("role", common.RoleAdminUser).Error)
	_, err = model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	visible, _, err := model.ListCustomerExportJobs(context.Background(), actor.Id, 1, 50)
	require.NoError(t, err)
	assert.Empty(t, visible)
	_, err = ResubmitUpstreamExportJob(actor.Id, job.JobID)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	require.NoError(t, model.SaveProviderURLGroupName(urlKey, "Primary upstream", actor.Id))
	adminJob, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, []int{9601}, urlKey)
	require.NoError(t, err)
	assert.NotEqual(t, completed.JobID, adminJob.JobID, "an admin must not reuse a file with root-only fields")
	adminClaimed, err := model.ClaimNextQueuedCustomerExportJob("upstream-admin", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, adminClaimed)
	runCustomerExportJob(context.Background(), adminClaimed, "upstream-admin", nil)
	adminCompleted, err := model.GetCustomerExportJobForOwner(adminJob.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, adminCompleted.Status)
	adminArtifact := adminCompleted.DecodeArtifact()
	require.NotNil(t, adminArtifact)
	require.Len(t, adminArtifact.Files, 1)
	assert.NotContains(t, string(store.uploaded[adminArtifact.Files[0].ObjectKey]), "task-root")

}

func TestUpstreamEvidenceExportFreezesGlobalCategoryAndResubmission(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}, &model.ProviderChannelBillingConfig{}))
	actor := model.User{Id: 9612, Username: "evidence-admin", AffCode: "ev-exp", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	facts := `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1,"cache_tokens":0,"cache_write_tokens":0,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":10}}`
	require.NoError(t, model.DB.Create(&[]model.Log{
		{Type: model.LogTypeConsume, ChannelId: 9612, CreatedAt: start + 1, Quota: 10, RequestId: "priced-test", TokenName: "模型测试", Content: "模型测试", Other: facts},
		{Type: model.LogTypeConsume, ChannelId: 9613, CreatedAt: start + 2, Quota: 10, RequestId: "ordinary", Other: `{}`},
	}).Error)
	publishBillingExportTestDay(t, start)
	request := CustomerExportRequest{StartTimestamp: start, EndTimestamp: end, UpstreamEvidenceFilter: "test_priced_rows", Language: "en"}
	job, err := SubmitUpstreamExportJob(context.Background(), actor.Id, request, nil, "")
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	require.NotNil(t, filters.Upstream)
	assert.Equal(t, "test_priced_rows", filters.Upstream.EvidenceFilter)
	assert.True(t, filters.Upstream.AllChannels)
	assert.Empty(t, filters.Upstream.ChannelIds)
	// Membership comes from the accepted ID boundary, including deleted channels.
	require.NoError(t, model.DB.Create(&model.Log{Type: model.LogTypeConsume, ChannelId: 9612, CreatedAt: start + 3, Quota: 10, RequestId: "late-test", TokenName: "模型测试", Content: "模型测试", Other: facts}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 9612, Name: "new name after acceptance"}).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("evidence-export", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "evidence-export", nil)
	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status)
	artifact := completed.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	records, err := csv.NewReader(strings.NewReader(string(store.uploaded[artifact.Files[0].ObjectKey]))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	row := reconciliationTestCells(t, upstreamDetailColumns, records[1])
	assert.Equal(t, "priced-test", row["request"])
	assert.Equal(t, "9612", row["channel_id"], "deleted channels remain included")
	assert.Empty(t, row["channel"], "new channel metadata must not change a frozen global export")
	assert.Equal(t, "1", row["discount"])
	resubmitted, err := ResubmitUpstreamExportJob(actor.Id, job.JobID)
	require.NoError(t, err)
	copyFilters, err := resubmitted.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, filters.Upstream, copyFilters.Upstream)
	_, err = model.CancelCustomerExportJob(resubmitted.JobID, actor.Id)
	require.NoError(t, err)
	all := CustomerExportRequest{StartTimestamp: start, EndTimestamp: end, ReportVersion: filters.BillingReportVersion, Language: "en"}
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, all, nil, "")
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound, "missing scope must not expand to all channels")
	all.UpstreamAllChannels = true
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, all, []int{9612}, "")
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest, "conflicting scopes must fail closed")
	allJob, err := SubmitUpstreamExportJob(context.Background(), actor.Id, all, nil, "")
	require.NoError(t, err)
	claimed, err = model.ClaimNextQueuedCustomerExportJob("evidence-export", time.Now().Add(time.Minute).Unix(), 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, allJob.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "evidence-export", nil)
	allCompleted, err := model.GetCustomerExportJobForOwner(allJob.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, allCompleted.Status)
	allArtifact := allCompleted.DecodeArtifact()
	require.NotNil(t, allArtifact)
	require.Len(t, allArtifact.Files, 1)
	allRecords, err := csv.NewReader(strings.NewReader(string(store.uploaded[allArtifact.Files[0].ObjectKey]))).ReadAll()
	require.NoError(t, err)
	require.Len(t, allRecords, 3)
	assert.Equal(t, "priced-test", reconciliationTestCells(t, upstreamDetailColumns, allRecords[1])["request"])
	assert.Equal(t, "ordinary", reconciliationTestCells(t, upstreamDetailColumns, allRecords[2])["request"])
	request.UpstreamEvidenceFilter = "typo"
	_, err = SubmitUpstreamExportJob(context.Background(), actor.Id, request, nil, "")
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest)
}
