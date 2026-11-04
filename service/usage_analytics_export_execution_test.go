package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageExportRunnerPublishesCurrentFormatAndRegenerates(t *testing.T) {
	for _, view := range []string{"self", "customer", "customers", "upstream"} {
		t.Run(view, func(t *testing.T) {
			truncate(t)
			store := setupCustomerExportServiceTest(t)
			require.NoError(t, model.DB.AutoMigrate(&model.BatchJob{}, &model.ProviderChannelBillingDiscount{}))
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 42).Update("role", common.RoleAdminUser).Error)
			target := 43
			request := UsageAnalyticsExportRequest{Period: "day", Date: "2026-10-12", View: view, Language: "en", UserId: &target}
			job, err := SubmitUsageAnalyticsExport(42, request)
			require.NoError(t, err)
			for attempt := 0; attempt < 2; attempt++ {
				claimed, err := model.ClaimNextQueuedCustomerExportJob("usage-test", common.GetTimestamp()+120, 1800)
				require.NoError(t, err)
				require.NotNil(t, claimed)
				runCustomerExportJob(context.Background(), claimed, "usage-test", nil)
				finished, err := model.GetCustomerExportJob(job.JobID)
				require.NoError(t, err)
				require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status, finished.ErrorCode)
				require.NotNil(t, finished.DecodeArtifact())
				assert.NotEmpty(t, store.uploaded)
				if attempt == 0 {
					job, err = ResubmitUsageAnalyticsExport(42, job.JobID, view == "self")
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestUsageExportRunnerRejectsUnsupportedFormat(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{Period: "day", Date: "2026-10-12", View: "self"})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	filters.FieldVersion = -1
	raw, err := common.Marshal(filters)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(job).Update("filters", string(raw)).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("format-test", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "format-test", nil)
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobStatusFailed, finished.Status)
	assert.Equal(t, "format_changed", finished.ErrorCode)
	assert.Nil(t, finished.DecodeArtifact())
	assert.Empty(t, store.uploaded)
}

func TestUsageExportModelScopeSurvivesExecutionAndRegeneration(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.BatchJob{}))
	period, err := model.ResolveUsageAnalyticsPeriod("day", "2026-10-12", common.GetTimestamp())
	require.NoError(t, err)
	for _, row := range []model.Log{
		{UserId: 42, TokenId: 1, Type: model.LogTypeConsume, CreatedAt: period.StartTimestamp + 1, ModelName: "selected", Quota: 100, PromptTokens: 10, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 42, TokenId: 2, Type: model.LogTypeConsume, CreatedAt: period.StartTimestamp + 2, ModelName: "selected", Quota: 200, PromptTokens: 20, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 42, TokenId: 1, Type: model.LogTypeConsume, CreatedAt: period.StartTimestamp + 3, ModelName: "selected-extra", Quota: 900, PromptTokens: 90, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	} {
		require.NoError(t, model.LOG_DB.Create(&row).Error)
	}
	request := UsageAnalyticsExportRequest{Period: "day", Date: "2026-10-12", View: "self", Language: "en", ModelName: "selected"}
	job, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	for attempt := 0; attempt < 2; attempt++ {
		filters, err := job.DecodeFilters()
		require.NoError(t, err)
		assert.Equal(t, "selected", filters.ModelName)
		claimed, err := model.ClaimNextQueuedCustomerExportJob("model-test", common.GetTimestamp()+120, 1800)
		require.NoError(t, err)
		require.NotNil(t, claimed)
		runCustomerExportJob(context.Background(), claimed, "model-test", nil)
		finished, err := model.GetCustomerExportJob(job.JobID)
		require.NoError(t, err)
		require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
		artifact := finished.DecodeArtifact()
		require.NotNil(t, artifact)
		require.Len(t, artifact.Files, 1)
		rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(store.uploaded[artifact.Files[0].ObjectKey], []byte(customerExportCsvBOM)))).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 6)
		for _, row := range rows[1 : len(rows)-1] {
			assert.Equal(t, "selected", row[6])
		}
		total := rows[len(rows)-1]
		assert.Equal(t, "grand_total", total[0])
		assert.Equal(t, "2", total[13])
		assert.Equal(t, "30", total[18])
		assert.Equal(t, "300", total[24])
		if attempt == 0 {
			job, err = ResubmitUsageAnalyticsExport(42, job.JobID, true)
			require.NoError(t, err)
		}
	}
}

func TestUsageExportRejectsInvalidModelScope(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	for _, tc := range []struct{ view, model string }{{"self", strings.Repeat("x", 256)}, {"upstream", "selected"}, {"customers", "selected"}} {
		request := UsageAnalyticsExportRequest{Period: "day", Date: "2026-10-12", View: tc.view, ModelName: tc.model}
		_, err := SubmitUsageAnalyticsExport(42, request)
		assert.ErrorIs(t, err, ErrCustomerExportInvalidRequest)
	}
}

func TestUsageExportModelWithoutMatchesPublishesEmptyResult(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.BatchJob{}))
	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{Period: "week", Date: "2026-10-12", View: "self", ModelName: "absent-model"})
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("empty-test", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "empty-test", nil)
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
	artifact := finished.DecodeArtifact()
	require.NotNil(t, artifact)
	assert.Empty(t, artifact.Files)
	assert.Empty(t, store.uploaded)
}

type unavailableUsageExportStore struct{ *stubExportStore }

func (s unavailableUsageExportStore) ExportPutFile(context.Context, string, string, string, int64) error {
	return errors.New("private upstream diagnostic must not reach the job view")
}

func TestUsageExportStorageFailureIsClassifiedWithoutPublishingPrivateErrors(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	customerExportStoreOverride = unavailableUsageExportStore{store}
	require.NoError(t, model.DB.AutoMigrate(&model.BatchJob{}))
	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{Period: "day", Date: "2026-10-12", View: "self"})
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("storage-test", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(context.Background(), claimed, "storage-test", nil)
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobStatusFailed, finished.Status)
	assert.Equal(t, "storage_unavailable", finished.ErrorCode)
	assert.Empty(t, finished.ToView().Error)
	assert.Nil(t, finished.ToView().Artifact)
	assert.Empty(t, store.uploaded)
}
