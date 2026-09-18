package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomerExportTestDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Log{}, &User{}))
	require.NoError(t, migrateCustomerExportDB())
	for _, id := range []int{1, 2, 3, 5, 7, 99} {
		require.NoError(t, db.Create(&User{Id: id, Username: fmt.Sprintf("user-%d", id), AffCode: fmt.Sprintf("a%d", id), Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
}

func customerExportTestFilters(start int64) CustomerExportFilters {
	return CustomerExportFilters{
		FieldVersion: 1, StartTimestamp: start, EndTimestamp: start + 3600, Timezone: "Asia/Shanghai",
	}
}

func TestCreateCustomerExportJobPerUserQuotaAndSlots(t *testing.T) {
	setupCustomerExportTestDB(t)

	first, created, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	assert.True(t, created)
	require.NotNil(t, first)

	// 本人相同申请返回原任务，不重复扫描。
	same, created, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.JobID, same.JobID)

	// 本人不同申请返回忙碌，不能绕过个人配额。
	_, _, err = CreateCustomerExportJob(1, 1, CustomerExportJobTypeStatementSummary, customerExportTestFilters(1000))
	assert.ErrorIs(t, err, ErrCustomerExportUserBusy)

	// 管理员更换目标客户也不能绕过按发起人计的配额。
	_, _, err = CreateCustomerExportJob(1, 42, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	assert.ErrorIs(t, err, ErrCustomerExportUserBusy)

	// 其他用户可以占满剩余槽位；队列满后快速拒绝。
	for userId := 2; userId <= CustomerExportSlotCount; userId++ {
		_, created, err := CreateCustomerExportJob(userId, userId, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
		require.NoError(t, err)
		assert.True(t, created)
	}
	_, _, err = CreateCustomerExportJob(99, 99, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	assert.ErrorIs(t, err, ErrCustomerExportQueueBusy)

	// 排队取消后原子释放槽位与配额，新申请可进入。
	cancelled, err := CancelCustomerExportJob(first.JobID, 1)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusCancelled, cancelled.Status)
	assert.Nil(t, cancelled.ActiveKey)

	job, created, err := CreateCustomerExportJob(99, 99, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, CustomerExportJobStatusQueued, job.Status)

	// 终态任务不可再次取消。
	_, err = CancelCustomerExportJob(first.JobID, 1)
	assert.ErrorIs(t, err, ErrCustomerExportNotCancellable)
}

func TestClaimAndFinishCustomerExportJob(t *testing.T) {
	setupCustomerExportTestDB(t)

	job, _, err := CreateCustomerExportJob(5, 5, CustomerExportJobTypeStatementDetails, customerExportTestFilters(2000))
	require.NoError(t, err)

	claimed, err := ClaimNextQueuedCustomerExportJob("exec-1", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, job.JobID, claimed.JobID)
	assert.Equal(t, CustomerExportJobStatusRunning, claimed.Status)

	// 认领后同用户相同申请仍返回原任务，不产生第二次扫描。
	duplicate, created, err := CreateCustomerExportJob(5, 5, CustomerExportJobTypeStatementDetails, customerExportTestFilters(2000))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, job.JobID, duplicate.JobID)

	// 认领后同用户不同申请返回忙碌。
	_, _, err = CreateCustomerExportJob(5, 6, CustomerExportJobTypeStatementDetails, customerExportTestFilters(2000))
	assert.ErrorIs(t, err, ErrCustomerExportUserBusy)

	artifact := &CustomerExportArtifact{
		Files:     []CustomerExportArtifactFile{{ObjectKey: "exports/jobs/x/1.csv", FileName: "1.csv", SizeBytes: 10, LineCount: 1, Sha256: "abc"}},
		LineCount: 1, GeneratedAt: 12345, ExpiresAt: 90000,
	}
	require.NoError(t, FinishCustomerExportJob(job.JobID, "exec-1", CustomerExportJobStatusSucceeded, "", "", artifact))

	// 重复完成被条件更新拒绝；产物与终态保持第一次的结果。
	err = FinishCustomerExportJob(job.JobID, "exec-1", CustomerExportJobStatusFailed, "internal", "", nil)
	assert.ErrorIs(t, err, ErrCustomerExportStateConflict)

	finished, err := GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusSucceeded, finished.Status)
	assert.Nil(t, finished.ActiveKey)
	require.NotNil(t, finished.DecodeArtifact())
	assert.Equal(t, int64(1), finished.DecodeArtifact().LineCount)
	assert.EqualValues(t, claimed.SlotId, finished.SlotId, "slot reference is retained as a historical fact")

	// 槽位已释放：新申请可受理。
	_, created, err = CreateCustomerExportJob(5, 5, CustomerExportJobTypeStatementDetails, customerExportTestFilters(2000))
	require.NoError(t, err)
	assert.True(t, created)
}

func TestCustomerExportQueueWaitTimeout(t *testing.T) {
	setupCustomerExportTestDB(t)

	stale, _, err := CreateCustomerExportJob(7, 7, CustomerExportJobTypeUsageLogs, customerExportTestFilters(3000))
	require.NoError(t, err)
	require.NoError(t, DB.Model(&CustomerExportJob{}).Where("job_id = ?", stale.JobID).
		Update("created_at", common.GetTimestamp()-3600).Error)

	claimed, err := ClaimNextQueuedCustomerExportJob("exec-1", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	assert.Nil(t, claimed, "expired queue wait must not be claimed")

	timedOut, err := GetCustomerExportJob(stale.JobID)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusFailed, timedOut.Status)
	assert.Equal(t, "queue_timeout", timedOut.ErrorCode)
	assert.Nil(t, timedOut.ActiveKey)
}

func TestRecoverInterruptedCustomerExportJobs(t *testing.T) {
	setupCustomerExportTestDB(t)

	job, _, err := CreateCustomerExportJob(8, 8, CustomerExportJobTypeUsageLogs, customerExportTestFilters(4000))
	require.NoError(t, err)
	require.NoError(t, DB.Model(&CustomerExportJob{}).Where("job_id = ?", job.JobID).Updates(map[string]any{
		"status": CustomerExportJobStatusRunning, "executor": "exec-old", "lease_until": common.GetTimestamp() - 1,
	}).Error)

	recovered, err := RecoverInterruptedCustomerExportJobs(common.GetTimestamp())
	require.NoError(t, err)
	assert.EqualValues(t, 1, recovered)

	after, err := GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusFailed, after.Status)
	assert.Equal(t, "lease_expired", after.ErrorCode)
}

func TestExpireCustomerExportArtifactsKeepsObjectRefsForCleanup(t *testing.T) {
	setupCustomerExportTestDB(t)

	job, _, err := CreateCustomerExportJob(9, 9, CustomerExportJobTypeUsageLogs, customerExportTestFilters(5000))
	require.NoError(t, err)
	claimed, err := ClaimNextQueuedCustomerExportJob("exec-1", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.NoError(t, FinishCustomerExportJob(job.JobID, "exec-1", CustomerExportJobStatusSucceeded, "", "", &CustomerExportArtifact{
		Files:       []CustomerExportArtifactFile{{ObjectKey: "exports/jobs/x/1.csv", FileName: "1.csv"}},
		GeneratedAt: 1, ExpiresAt: common.GetTimestamp() - 1,
	}))

	expired, err := ExpireCustomerExportArtifacts(common.GetTimestamp())
	require.NoError(t, err)
	require.Len(t, expired, 1)

	after, err := GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, CustomerExportJobStatusExpired, after.Status)
	assert.NotNil(t, after.DecodeArtifact(), "artifact refs must survive until physical cleanup")

	require.NoError(t, DeleteCustomerExportArtifactRecord(job.JobID))
	after, err = GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Nil(t, after.DecodeArtifact())
}

func TestLostExportReservationDoesNotBlockOtherCustomers(t *testing.T) {
	for _, missingSlotID := range []bool{false, true} {
		t.Run(fmt.Sprint(missingSlotID), func(t *testing.T) {
			setupCustomerExportTestDB(t)
			first, _, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
			require.NoError(t, err)
			second, _, err := CreateCustomerExportJob(2, 2, CustomerExportJobTypeUsageLogs, customerExportTestFilters(1000))
			require.NoError(t, err)
			if missingSlotID {
				require.NoError(t, DB.Model(first).Update("slot_id", 0).Error)
			} else {
				require.NoError(t, DB.Model(&CustomerExportSlot{}).Where("job_id = ?", first.JobID).Update("job_id", "").Error)
			}
			claimed, err := ClaimNextQueuedCustomerExportJob("executor", common.GetTimestamp()+120, 1800)
			require.NoError(t, err)
			require.NotNil(t, claimed)
			assert.Equal(t, second.JobID, claimed.JobID)
			saved, err := GetCustomerExportJob(first.JobID)
			require.NoError(t, err)
			assert.Equal(t, CustomerExportJobStatusFailed, saved.Status)
			assert.Equal(t, "slot_lost", saved.ErrorCode)
			assert.Nil(t, saved.ActiveKey)
			var slot CustomerExportSlot
			require.NoError(t, DB.Where("job_id = ?", second.JobID).First(&slot).Error)
		})
	}
}
