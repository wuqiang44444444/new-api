package model

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskRevisionFailureFencesFirstExport(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}, &User{}))
	ctx := context.Background()
	filters := customerExportTestFilters(1000)
	before, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	require.NotEmpty(t, before)
	var count int64
	require.NoError(t, db.Model(&BillingStatementRetention{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:task_revision_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_statement_revisions" {
			tx.AddError(errors.New("revision unavailable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove("test:task_revision_failure") })
	task := Task{UserId: 1, AppID: 40, TaskID: "cross-month-task", CreatedAt: 1785513700, Quota: 100}
	require.NoError(t, db.Create(&task).Error, "a revision failure must not discard settlement facts")
	var stored Task
	require.NoError(t, db.First(&stored, task.ID).Error)
	assert.Equal(t, task.Quota, stored.Quota)
	after, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.Empty(t, after, "a task can invalidate an older month's export before any retention row exists")
	retention, err := GetBillingStatementRetention(ctx, 1, naturalMonthStartAt(task.CreatedAt))
	require.NoError(t, err)
	require.NotNil(t, retention)
	assert.Equal(t, BillingStatementRetentionPartial, retention.Status)
}

func TestCustomerStatementExportReuse(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, migrateCustomerExportDB())
	require.NoError(t, DB.Create(&User{Id: 1, Username: "customer", AffCode: "customer", Status: common.UserStatusEnabled, Role: common.RoleAdminUser}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "admin", AffCode: "admin", Status: common.UserStatusEnabled, Role: common.RoleAdminUser}).Error)
	filters := customerExportTestFilters(1000)
	ctx := context.Background()
	source, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	require.NotEmpty(t, source)
	reuse := CustomerExportReuse{SourceVersion: source, StoreIdentity: "store-a"}
	job, created, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeStatementSummary, filters, reuse)
	require.NoError(t, err)
	require.True(t, created)
	claimed, err := ClaimNextQueuedCustomerExportJob("runner", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.Equal(t, job.JobID, claimed.JobID)
	require.NoError(t, FinishCustomerExportJob(job.JobID, "runner", CustomerExportJobStatusSucceeded, "", "", &CustomerExportArtifact{
		SourceVersion: source, StoreIdentity: "store-a", Files: []CustomerExportArtifactFile{}, ExpiresAt: common.GetTimestamp() + 3600,
	}))
	same, created, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeStatementSummary, filters, reuse)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, job.JobID, same.JobID)
	assert.Empty(t, same.ToView().Artifact.SourceVersion, "source proofs are private")
	var count int64
	require.NoError(t, DB.Model(&CustomerExportJob{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, DB.Model(&CustomerExportSlot{}).Where("job_id <> ?", "").Count(&count).Error)
	assert.Zero(t, count, "reuse never occupies the queue")

	for _, tc := range []struct {
		name          string
		actor, target int
		filters       CustomerExportFilters
		reuse         CustomerExportReuse
	}{
		{"different owner", 2, 1, filters, reuse},
		{"different customer", 1, 2, filters, reuse},
		{"different month", 1, 1, customerExportTestFilters(3000), reuse},
		{"different language", 1, 1, func() CustomerExportFilters { f := filters; f.Language = "en"; return f }(), reuse},
		{"different currency", 1, 1, func() CustomerExportFilters { f := filters; f.Currency = "CNY"; f.CurrencyRate = 7; return f }(), reuse},
		{"different store", 1, 1, filters, CustomerExportReuse{SourceVersion: source, StoreIdentity: "store-b"}},
		{"unverified source", 1, 1, filters, CustomerExportReuse{StoreIdentity: "store-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, created, err := CreateCustomerExportJob(tc.actor, tc.target, CustomerExportJobTypeStatementSummary, tc.filters, tc.reuse)
			require.NoError(t, err)
			assert.True(t, created)
			assert.NotEqual(t, job.JobID, next.JobID)
			_, err = CancelCustomerExportJob(next.JobID, tc.actor)
			require.NoError(t, err)
		})
	}

	log := Log{UserId: 1, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 1, Quota: 10}
	require.NoError(t, DB.Create(&log).Error)
	changed, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, source, changed)
	next, created, err := CreateCustomerExportJob(1, 1, CustomerExportJobTypeStatementSummary, filters, CustomerExportReuse{SourceVersion: changed, StoreIdentity: "store-a"})
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, job.JobID, next.JobID)
	_, err = CancelCustomerExportJob(next.JobID, 1)
	require.NoError(t, err)

	require.NoError(t, DB.Model(&Log{}).Where("id = ?", log.Id).Update("quota", 20).Error)
	corrected, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, changed, corrected, "in-place corrections invalidate exports even with the same log count and ID")

	require.NoError(t, DB.Model(&CustomerExportJob{}).Where("job_id = ?", job.JobID).Update("expires_at", common.GetTimestamp()-1).Error)
	_, created, err = CreateCustomerExportJob(1, 1, CustomerExportJobTypeStatementSummary, filters, reuse)
	require.NoError(t, err)
	assert.True(t, created, "expired deliverables must be regenerated")
}

func TestCustomerStatementExportSourceRevisionBoundaries(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
	filters := customerExportTestFilters(1000)
	ctx := context.Background()
	first, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&Log{UserId: 2, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 1, Quota: 10}).Error)
	second, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.Equal(t, first, second, "another customer's usage does not invalidate this export")
	require.NoError(t, DB.Create(&User{Id: 1, Username: "renamed", AffCode: "renamed"}).Error)
	renamed, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, second, renamed)
	maintenance, err := BeginBillingStatementMaintenance(ctx, "repair", 7)
	require.NoError(t, err)
	duringMaintenance, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.Empty(t, duringMaintenance)
	_, err = EndBillingStatementMaintenance(ctx, maintenance.Generation, "verified fixture", 7)
	require.NoError(t, err)
	afterMaintenance, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, renamed, afterMaintenance)
	require.NoError(t, DB.AutoMigrate(&Task{}))
	task := Task{UserId: 1, TaskID: "cross-month-evidence", SubmitTime: 1}
	require.NoError(t, DB.Create(&task).Error)
	withTask, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, afterMaintenance, withTask, "task evidence may affect refunds in a different month")
	require.NoError(t, DB.Model(&Task{}).Where("id = ?", task.ID).Update("private_data", `{"billing_state":"settled"}`).Error)
	changedTask, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, withTask, changedTask)
	require.NoError(t, DB.Create(&BillingStatementRetention{UserId: 1, PeriodStart: 1000, Status: BillingStatementRetentionPartial}).Error)
	partial, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.Empty(t, partial, "incomplete source retention cannot prove reuse is safe")
	LOG_DB = DB.Session(&gorm.Session{})
	untracked, err := CustomerStatementExportSourceVersion(ctx, 1, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.Empty(t, untracked, "split databases cannot claim a transactional source revision")
}
