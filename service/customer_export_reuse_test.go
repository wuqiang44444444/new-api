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

// The storage boundary can change a real source row during generation, without
// mocking the revision proof or the submission/worker path being protected.
type changingStatementExportStore struct {
	*stubExportStore
	change func() error
}

func (s *changingStatementExportStore) ExportPutFile(ctx context.Context, key, mime, path string, size int64) error {
	if err := s.stubExportStore.ExportPutFile(ctx, key, mime, path, size); err != nil {
		return err
	}
	return s.change()
}

func TestCustomerStatementExportWorkerReuse(t *testing.T) {
	for _, jobType := range []string{model.CustomerExportJobTypeStatementSummary, model.CustomerExportJobTypeStatementDetails} {
		t.Run(jobType, func(t *testing.T) {
			db := setupVersionServiceTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
			store := setupCustomerExportServiceTest(t)
			require.NoError(t, model.InitBillingStatementSourceTracking())
			const start = int64(1785513600)
			log := model.Log{UserId: 42, TokenId: 7, TokenName: "key", ModelName: "test-model", Type: model.LogTypeConsume, CreatedAt: start + 100, Quota: 100, PromptTokens: 10, RequestId: "req-export", Other: `{"group_ratio":0.5}`}
			require.NoError(t, db.Create(&log).Error)
			request := CustomerExportRequest{JobType: jobType, StartTimestamp: start, EndTimestamp: start + 31*86400, Language: "en"}
			job, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			active, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			assert.Equal(t, job.JobID, active.JobID)
			claimed, err := model.ClaimNextQueuedCustomerExportJob("reuse-worker", common.GetTimestamp()+120, 1800)
			require.NoError(t, err)
			runCustomerExportJob(context.Background(), claimed, "reuse-worker", nil)
			finished, err := model.GetCustomerExportJob(job.JobID)
			require.NoError(t, err)
			require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
			require.NotNil(t, finished.DecodeArtifact())
			assert.NotEmpty(t, finished.DecodeArtifact().SourceVersion)
			artifact := finished.DecodeArtifact()
			require.Len(t, artifact.Files, 1)
			csvRows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[artifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
			require.NoError(t, err)
			require.Len(t, csvRows, 2)
			values := map[string]string{}
			for i, label := range csvRows[0] {
				values[label] = csvRows[1][i]
			}
			if jobType == model.CustomerExportJobTypeStatementSummary {
				assert.Equal(t, "statement-summary.csv", artifact.Files[0].FileName)
				assert.Equal(t, "test-model", values["Model"])
				assert.NotContains(t, values, "Request ID")
				assert.NotEmpty(t, values["Estimated savings"])
			} else {
				assert.Equal(t, "statement-details.csv", artifact.Files[0].FileName)
				assert.Equal(t, "req-export", values["Request ID"])
				assert.Equal(t, "0.5", values["Final factor"])
				assert.Equal(t, "Not recorded", values["Contract status"])
			}

			reused, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			assert.Equal(t, job.JobID, reused.JobID)
			var count int64
			require.NoError(t, db.Model(&model.CustomerExportJob{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)

			require.NoError(t, db.Model(&model.Log{}).Where("id = ?", log.Id).Update("quota", 200).Error)
			next, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			assert.NotEqual(t, job.JobID, next.JobID)
			customerExportStoreOverride = &changingStatementExportStore{store, func() error {
				return db.Model(&model.Log{}).Where("id = ?", log.Id).Update("quota", 300).Error
			}}
			claimed, err = model.ClaimNextQueuedCustomerExportJob("reuse-worker", common.GetTimestamp()+120, 1800)
			require.NoError(t, err)
			runCustomerExportJob(context.Background(), claimed, "reuse-worker", nil)
			changedDuringGeneration, err := model.GetCustomerExportJob(next.JobID)
			require.NoError(t, err)
			require.Equal(t, model.CustomerExportJobStatusSucceeded, changedDuringGeneration.Status)
			assert.Empty(t, changedDuringGeneration.DecodeArtifact().SourceVersion)
			third, err := SubmitCustomerExportJob(42, 42, request)
			require.NoError(t, err)
			assert.NotEqual(t, next.JobID, third.JobID, "a mixed source revision cannot be auto-reused")
		})
	}
}
