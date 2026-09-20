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
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}))
	actor := model.User{Id: 9601, Username: "upstream-export-admin", AffCode: "up-export", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 9601, Name: "=channel"}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, billingExportTimezone()).Unix()
	discount := model.ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: 9601, Discount: decimal.RequireFromString("0.12345678"), Reason: "confirmed supplier coefficient"}
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 0, actor.Id))
	t.Cleanup(func() { model.DB.Where("channel_id = ?", 9601).Delete(&model.ProviderChannelBillingDiscount{}) })
	request := CustomerExportRequest{StartTimestamp: start, EndTimestamp: end, RequestId: "export-match", Language: "en"}
	_, err := SubmitUpstreamExportJob(42, request, []int{9601}, "")
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	_, err = SubmitCustomerExportJob(actor.Id, 42, CustomerExportRequest{JobType: model.CustomerExportJobTypeUpstreamDetails, StartTimestamp: start, EndTimestamp: end})
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest, "single-customer endpoint must not admit the cross-customer type")
	job, err := SubmitUpstreamExportJob(actor.Id, request, []int{9601}, "")
	require.NoError(t, err)
	// Settings and coefficients changing after acceptance never alter the file.
	discount.Discount = decimal.RequireFromString("0.9")
	require.NoError(t, model.SaveProviderChannelBillingDiscount(&discount, 1, actor.Id))
	facts := `{"contract_applicable":false,"group_ratio":0.5,"model_ratio":1,"task_id":"task-public","upstream_task_id":"task-root"}`
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 500000, RequestId: "export-match", UpstreamRequestId: "up-1", Other: facts},
		{UserId: 43, ChannelId: 9601, Type: model.LogTypeRefund, CreatedAt: start + 2, Quota: 100000, RequestId: "export-match", UpstreamRequestId: "up-2", Other: facts},
		{UserId: 43, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + 3, Quota: 999, RequestId: "excluded", Other: facts},
	}).Error)
	// Cross both the UI page size and the reader's 500-row batch boundary.
	more := make([]model.Log, 500)
	for i := range more {
		more[i] = model.Log{UserId: 43, ChannelId: 9601, Type: model.LogTypeConsume, CreatedAt: start + int64(i) + 10, Quota: 1, RequestId: "export-match", Other: facts}
	}
	require.NoError(t, model.DB.Create(&more).Error)
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
	first := records[1]
	second := records[2]
	assert.Equal(t, "'=channel", first[8])
	assert.Equal(t, "1000000", first[18])
	assert.Equal(t, "-200000", second[18])
	assert.Equal(t, "0.12345678", first[24])
	assert.Equal(t, "1", first[25])
	assert.Equal(t, "up-1", first[29])
	assert.Equal(t, "task-root", first[31])
	assert.Equal(t, "refund", second[13])
	filters, err := completed.DecodeFilters()
	require.NoError(t, err)
	scope := customerExportScopeColumns{QuotaPerUnit: filters.QuotaPerUnit, Currency: filters.Currency, CurrencyRate: filters.CurrencyRate}
	assert.Equal(t, exportCurrencyAmount("123456.78", scope), first[23])
	assert.Equal(t, exportCurrencyAmount("-24691.356", scope), second[23])
	_, err = model.GetCustomerExportJobForOwner(job.JobID, 43)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	// A root-only file cannot be fetched after demotion, even to administrator.
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actor.Id).Update("role", common.RoleAdminUser).Error)
	_, err = model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	visible, err := model.ListCustomerExportJobs(actor.Id, 50)
	require.NoError(t, err)
	assert.Empty(t, visible)
	_, err = ResubmitUpstreamExportJob(actor.Id, job.JobID)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	adminJob, err := SubmitUpstreamExportJob(actor.Id, request, []int{9601}, "")
	require.NoError(t, err)
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
