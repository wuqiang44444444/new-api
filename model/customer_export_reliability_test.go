package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportRevokedAdminCannotAccessCustomerArtifact(t *testing.T) {
	setupCustomerExportTestDB(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1).Update("role", common.RoleAdminUser).Error)
	job, _, err := CreateCustomerExportJob(1, 2, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	_, err = GetCustomerExportJobForOwner(job.JobID, 1)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1).Update("role", common.RoleCommonUser).Error)
	_, err = GetCustomerExportJobForOwner(job.JobID, 1)
	assert.ErrorIs(t, err, ErrCustomerExportNotFound)
	_, err = CancelCustomerExportJob(job.JobID, 1)
	assert.ErrorIs(t, err, ErrCustomerExportNotFound)
	jobs, err := ListCustomerExportJobs(1, 50)
	require.NoError(t, err)
	assert.Empty(t, jobs)
	claimed, err := ClaimNextQueuedCustomerExportJob("exec", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	err = CheckCustomerExportExecution(context.Background(), claimed.JobID, "exec", common.GetTimestamp())
	assert.ErrorIs(t, err, ErrCustomerExportNotFound)
}

func TestCustomerExportCancelledCannotPublishAndExpiredCancellationRecovers(t *testing.T) {
	setupCustomerExportTestDB(t)
	job, _, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	_, err = ClaimNextQueuedCustomerExportJob("exec", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	cancelled, err := CancelCustomerExportJob(job.JobID, 1)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusCancelWait, cancelled.Status)
	assert.True(t, cancelled.CancelRequested)
	assert.NotNil(t, cancelled.ActiveKey)
	_, _, err = CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(2000))
	assert.ErrorIs(t, err, ErrCustomerExportUserBusy)
	again, err := CancelCustomerExportJob(job.JobID, 1)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusCancelWait, again.Status)
	assert.ErrorIs(t, FinishCustomerExportJob(job.JobID, "exec", CustomerExportJobStatusSucceeded, "", "", &CustomerExportArtifact{}), ErrCustomerExportStateConflict)
	require.NoError(t, DB.Model(&CustomerExportJob{}).Where("job_id = ?", job.JobID).Update("lease_until", common.GetTimestamp()-1).Error)
	count, err := RecoverInterruptedCustomerExportJobs(common.GetTimestamp())
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	_, created, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(2000))
	require.NoError(t, err)
	assert.True(t, created)
}

func TestCustomerExportExpiredWorkerCannotRenewOrPublish(t *testing.T) {
	setupCustomerExportTestDB(t)
	job, _, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	_, err = ClaimNextQueuedCustomerExportJob("old", common.GetTimestamp()-1, 1800)
	require.NoError(t, err)
	assert.ErrorIs(t, RenewCustomerExportLease(job.JobID, "old", common.GetTimestamp()+120), ErrCustomerExportStateConflict)
	assert.ErrorIs(t, FinishCustomerExportJob(job.JobID, "old", CustomerExportJobStatusSucceeded, "", "", &CustomerExportArtifact{}), ErrCustomerExportStateConflict)
}

func TestCustomerExportPendingObjectsSurviveFailedJobRetention(t *testing.T) {
	setupCustomerExportTestDB(t)
	job, _, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	_, err = ClaimNextQueuedCustomerExportJob("exec", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	artifact := &CustomerExportArtifact{Files: []CustomerExportArtifactFile{{ObjectKey: "exports/jobs/pending/file.csv"}}, StoreIdentity: "test"}
	require.NoError(t, StageCustomerExportArtifact(context.Background(), job.JobID, artifact))
	staged, err := GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Nil(t, staged.ToView().Artifact)
	require.NoError(t, FinishCustomerExportJob(job.JobID, "exec", CustomerExportJobStatusFailed, "upload_failed", "", nil))
	require.NoError(t, DB.Model(&CustomerExportJob{}).Where("job_id = ?", job.JobID).Update("finished_at", 1000).Error)
	n, err := CleanupCustomerExportJobRecords(common.GetTimestamp(), 60)
	require.NoError(t, err)
	assert.Zero(t, n)
	pending, err := FindExpiredCustomerExportArtifacts(5)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, DeleteCustomerExportArtifactRecord(job.JobID))
	n, err = CleanupCustomerExportJobRecords(common.GetTimestamp(), 60)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

func TestCustomerStatementIncompleteContractDoesNotInventSavings(t *testing.T) {
	setupBillingReconciliationTestDB(t)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 7, TokenId: 4, Type: LogTypeConsume, ModelName: "m", CreatedAt: 1000, Quota: 100, Other: `{"group_ratio":0.5,"model_ratio":1,"contract_id":7}`}).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 900, 1100, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, statement.Summary.NetQuota)
	assert.Nil(t, statement.OriginalQuota)
	assert.Nil(t, statement.DiscountQuota)
	require.Len(t, statement.DiscountCombinations, 1)
	assert.False(t, statement.DiscountCombinations[0].OriginalKnown)
}

func TestCustomerExportManualRefundCrossPeriodDuplicateAndAuxiliaryFacts(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "auxiliary preserved", true: "cross period duplicate"}[duplicate], func(t *testing.T) {
			setupBillingReconciliationTestDB(t)
			original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: 800, Quota: 87, Other: `{"group_ratio":0.87,"contract_discount":0.5,"contract_id":3,"contract_name":"annual","contract_version":2,"fee_quota":7,"statement_snapshot":{"billing_mode":"per_second"}}`}
			require.NoError(t, LOG_DB.Create(&original).Error)
			other, err := common.Marshal(map[string]any{"admin_info": map[string]any{"original_preauth_log_id": original.Id}})
			require.NoError(t, err)
			refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1000, Quota: 87, Other: string(other)}
			require.NoError(t, LOG_DB.Create(&refund).Error)
			if duplicate {
				extra := refund
				extra.Id = 0
				extra.CreatedAt = 2000
				extra.Quota = 1
				require.NoError(t, LOG_DB.Create(&extra).Error)
			}
			batch, err := NextCustomerExportLogBatch(context.Background(), CustomerExportBatchParams{UserId: 7, StartTimestamp: 900, EndTimestamp: 1100, Limit: 10})
			require.NoError(t, err)
			rows, err := BuildCustomerExportRows(context.Background(), batch, "", "", nil)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			if duplicate {
				assert.Equal(t, BillingReconciliationModeUnknown, rows[0].BillingMode)
			} else {
				assert.Equal(t, BillingReconciliationModePerSecond, rows[0].BillingMode)
				assert.True(t, rows[0].HasAuxiliaryCharge)
				assert.Equal(t, "annual", rows[0].ContractName)
				assert.EqualValues(t, 2, rows[0].ContractVersion)
			}
			assert.Empty(t, rows[0].OriginalEstimate)
		})
	}
}
