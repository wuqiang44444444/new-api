package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementExportsWithoutPublishedDatesReportMissingCoverage(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	zone := billingExportTimezone()
	month := time.Now().In(zone).AddDate(0, -2, 0)
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 1, 0)
	require.NoError(t, db.Create(&model.Log{UserId: 42, ModelName: "unpublished", CreatedAt: start.Unix() + 1, Type: model.LogTypeConsume, Quota: 100}).Error)

	for _, jobType := range []string{model.CustomerExportJobTypeStatementSummary, model.CustomerExportJobTypeStatementDetails} {
		t.Run(jobType, func(t *testing.T) {
			request := CustomerExportRequest{JobType: jobType, StartTimestamp: start.Unix(), EndTimestamp: end.Unix(), Language: "en"}
			job, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			claimed, err := model.ClaimNextQueuedCustomerExportJob("coverage-runner", common.GetTimestamp()+120, 1800)
			require.NoError(t, err)
			require.NotNil(t, claimed)
			require.Equal(t, job.JobID, claimed.JobID)
			runCustomerExportJob(t.Context(), claimed, "coverage-runner", nil)
			finished, err := model.GetCustomerExportJob(job.JobID)
			require.NoError(t, err)
			require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)

			artifact := finished.ToView().Artifact
			require.NotNil(t, artifact)
			require.Len(t, artifact.MissingDates, int(end.Sub(start).Hours()/24))
			assert.Equal(t, start.Format(time.DateOnly), artifact.MissingDates[0])
			assert.Empty(t, artifact.Files)
			assert.Empty(t, store.uploaded)
			reused, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			assert.Equal(t, job.JobID, reused.JobID)
		})
	}
}
