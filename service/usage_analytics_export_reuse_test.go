package service

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfUsageExportReusesOnlyTheSameUnexpiredReport(t *testing.T) {
	store := setupCustomerExportServiceTest(t)
	const date = "2026-09-16"
	period, err := model.ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: 42, Type: model.LogTypeConsume, CreatedAt: period.StartTimestamp + 1,
		ModelName: "reuse-usage-model", Quota: 10,
	}).Error)
	t.Cleanup(func() {
		model.LOG_DB.Where("user_id = ? AND model_name = ?", 42, "reuse-usage-model").Delete(&model.Log{})
	})
	publishUsageExportTestDays(t, date)
	request := UsageAnalyticsExportRequest{Period: "day", Date: date, View: "self", Language: "en", ModelName: "reuse-usage-model"}
	first, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)

	queued, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, queued.JobID)

	claimed, err := model.ClaimNextQueuedCustomerExportJob("usage-reuse", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.Equal(t, first.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "usage-reuse", nil)

	same, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, same.JobID)
	assert.Equal(t, model.CustomerExportJobStatusSucceeded, same.Status)
	require.Len(t, store.uploaded, 1)

	differentFilter := request
	differentFilter.ModelName = ""
	filtered, err := SubmitUsageAnalyticsExport(42, differentFilter)
	require.NoError(t, err)
	assert.NotEqual(t, first.JobID, filtered.JobID)
	_, err = model.CancelCustomerExportJob(filtered.JobID, 42)
	require.NoError(t, err)

	require.NoError(t, model.DB.Model(&model.CustomerExportJob{}).Where("job_id = ?", first.JobID).
		Update("expires_at", common.GetTimestamp()-1).Error)
	replacement, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	assert.NotEqual(t, first.JobID, replacement.JobID)
	_, err = model.CancelCustomerExportJob(replacement.JobID, 42)
	require.NoError(t, err)

	// Restore the first file's validity to isolate the report-generation check.
	require.NoError(t, model.DB.Model(&model.CustomerExportJob{}).Where("job_id = ?", first.JobID).
		Update("expires_at", same.ExpiresAt).Error)
	require.NoError(t, model.BuildUsageReportDay(context.Background(), date, "export-fixture", "runner", nil))
	updated, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	assert.NotEqual(t, first.JobID, updated.JobID)
	assert.Equal(t, model.CustomerExportJobStatusQueued, updated.Status)
}

func TestSelfUsageExportReusesAnEmptyResult(t *testing.T) {
	store := setupCustomerExportServiceTest(t)
	const date = "2026-09-16"
	publishUsageExportTestDays(t, date)
	request := UsageAnalyticsExportRequest{Period: "day", Date: date, View: "self", Language: "en", ModelName: "model-with-no-usage"}
	first, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("empty-usage-reuse", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.Equal(t, first.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "empty-usage-reuse", nil)

	same, err := SubmitUsageAnalyticsExport(42, request)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, same.JobID)
	assert.Equal(t, model.CustomerExportJobStatusSucceeded, same.Status)
	require.NotNil(t, same.DecodeArtifact())
	assert.Empty(t, same.DecodeArtifact().Files)
	assert.Empty(t, store.uploaded)
}

func TestAdminUsageOverviewExportReuseTracksSearchMembership(t *testing.T) {
	setupCustomerExportServiceTest(t)
	const date = "2026-09-16"
	const actorID = 42
	const targetID = 43
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleAdminUser).Error)
	t.Cleanup(func() {
		model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleCommonUser)
		model.DB.Model(&model.User{}).Where("id = ?", targetID).Update("display_name", "")
		model.LOG_DB.Where("user_id = ? AND model_name = ?", targetID, "admin-reuse-model").Delete(&model.Log{})
	})
	period, err := model.ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: targetID, Type: model.LogTypeConsume, CreatedAt: period.StartTimestamp + 1,
		ModelName: "admin-reuse-model", Quota: 10,
	}).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", targetID).Update("display_name", "admin-reuse-search").Error)
	publishUsageExportTestDays(t, date)
	request := UsageAnalyticsExportRequest{Period: "day", Date: date, View: "customers", Language: "en", Search: "admin-reuse-search"}

	first, err := SubmitUsageAnalyticsExport(actorID, request)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("admin-usage-reuse", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.Equal(t, first.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "admin-usage-reuse", nil)
	reused, err := SubmitUsageAnalyticsExport(actorID, request)
	require.NoError(t, err)
	require.Equal(t, first.JobID, reused.JobID)
	assert.Equal(t, model.CustomerExportJobStatusSucceeded, reused.Status)

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", targetID).Update("display_name", "no-longer-matching").Error)
	next, err := SubmitUsageAnalyticsExport(actorID, request)
	require.NoError(t, err)
	assert.NotEqual(t, first.JobID, next.JobID)
	assert.Equal(t, model.CustomerExportJobStatusQueued, next.Status)
}

func TestAdminCustomerUsageExportRequiresAdminEvenForTheirOwnAccount(t *testing.T) {
	setupCustomerExportServiceTest(t)
	const actorID = 42
	const date = "2026-09-16"
	request := UsageAnalyticsExportRequest{Period: "day", Date: date, View: "customer", UserId: common.GetPointer(actorID)}
	_, err := SubmitUsageAnalyticsExport(actorID, request)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleAdminUser).Error)
	t.Cleanup(func() { model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleCommonUser) })
	publishUsageExportTestDays(t, date)

	first, err := SubmitUsageAnalyticsExport(actorID, request)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("admin-own-usage", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.Equal(t, first.JobID, claimed.JobID)
	runCustomerExportJob(context.Background(), claimed, "admin-own-usage", nil)
	same, err := SubmitUsageAnalyticsExport(actorID, request)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, same.JobID)
	assert.Equal(t, model.CustomerExportJobStatusSucceeded, same.Status)

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleCommonUser).Error)
	_, err = model.GetCustomerExportJobForOwner(first.JobID, actorID)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
}

func TestAdminCustomerUsageCSVSupportsNaturalMonthForBothScopes(t *testing.T) {
	store := setupCustomerExportServiceTest(t)
	const actorID, customerID = 42, 43
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleAdminUser).Error)
	t.Cleanup(func() {
		model.DB.Model(&model.User{}).Where("id = ?", actorID).Update("role", common.RoleCommonUser)
		model.LOG_DB.Where("user_id = ? AND model_name = ?", customerID, "month-export-model").Delete(&model.Log{})
	})
	period, err := model.ResolveUsageAnalyticsPeriod("month", "2026-09-16", common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: customerID, Type: model.LogTypeConsume, CreatedAt: period.EndTimestamp - 10,
		ModelName: "month-export-model", Quota: 10, RequestId: "month-export",
	}).Error)
	publishUsageExportTestDays(t, "2026-09-01", "2026-09-30")
	for _, view := range []string{"customers", "customer"} {
		t.Run(view, func(t *testing.T) {
			request := UsageAnalyticsExportRequest{Period: "month", Date: "2026-09-16", View: view, UserId: common.GetPointer(customerID)}
			job, err := SubmitUsageAnalyticsExport(actorID, request)
			require.NoError(t, err)
			filters, err := job.DecodeFilters()
			require.NoError(t, err)
			assert.Equal(t, period.StartTimestamp, filters.StartTimestamp)
			assert.Equal(t, period.EndTimestamp, filters.EndTimestamp)
			claimed, err := model.ClaimNextQueuedCustomerExportJob("admin-month", common.GetTimestamp()+120, 1800)
			require.NoError(t, err)
			require.Equal(t, job.JobID, claimed.JobID)
			runCustomerExportJob(context.Background(), claimed, "admin-month", nil)
			finished, err := model.GetCustomerExportJob(job.JobID)
			require.NoError(t, err)
			require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
			files := finished.DecodeArtifact().Files
			require.Len(t, files, 1)
			content := strings.TrimPrefix(string(store.uploaded[files[0].ObjectKey]), "\uFEFF")
			rows, err := csv.NewReader(strings.NewReader(content)).ReadAll()
			require.NoError(t, err)
			var days []string
			for _, row := range rows[1:] {
				if row[0] == "daily" {
					days = append(days, row[1])
				}
			}
			assert.Contains(t, days, "2026-09-30")
			assert.NotContains(t, days, "2026-10-01")
		})
	}
	upstream, err := SubmitUsageAnalyticsExport(actorID, UsageAnalyticsExportRequest{Period: "month", Date: "2026-09-16", View: "upstream"})
	require.NoError(t, err)
	filters, err := upstream.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, period.StartTimestamp, filters.StartTimestamp)
	assert.Equal(t, period.EndTimestamp, filters.EndTimestamp)
	assert.Equal(t, usageAnalyticsExportFieldVersion, filters.FieldVersion)
}
