package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type recoveryFixture struct {
	db   *gorm.DB
	path string
	args []string
	task model.Task
}

func newRecoveryFixture(t *testing.T, subscription bool) *recoveryFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.UserSubscription{}, &model.Task{}, &model.TaskBillingDelivery{}, &model.Log{}, &model.AuditLog{}, &model.QuotaData{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "operator", AffCode: "operator", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "customer", AffCode: "customer", Quota: 900, UsedQuota: 100}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 3, UserId: 2, Key: "fixture-token", RemainQuota: 900, UsedQuota: 100}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 4, UsedQuota: 100}).Error)
	task := model.Task{TaskID: "task_fixture", UserId: 2, AppID: 5, ChannelId: 4, Quota: 100, Status: model.TaskStatusReconciliationRequired, BillingState: model.TaskBillingStatePending, Progress: "50%", FailReason: "upstream_contract_violation: untrusted Synlink task identity",
		Properties: model.Properties{OriginModelName: "customer-video", UpstreamModelName: "private-model"},
		PrivateData: model.TaskPrivateData{TokenId: 3, BillingSource: "wallet", UpstreamTaskID: "fixture-provider-task", VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1,
			Execution:    &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: "seedance-link", Version: "1.3.3", APIVersion: 3}},
			AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
	}
	if subscription {
		require.NoError(t, db.Create(&model.UserSubscription{Id: 6, UserId: 2, AmountTotal: 1000, AmountUsed: 100, Status: "active"}).Error)
		task.PrivateData.BillingSource, task.PrivateData.SubscriptionId = "subscription", 6
	}
	require.NoError(t, db.Create(&task).Error)
	return &recoveryFixture{db: db, path: path, task: task, args: []string{"-database", path, "-task-id", "task_fixture", "-user-id", "2", "-app-id", "5", "-channel-id", "4", "-operator-id", "1", "-expected-quota", "100", "-evidence-ref", "evidence_fixture"}}
}

func (f *recoveryFixture) apply(t *testing.T, backupName string) error {
	t.Helper()
	args := append(append([]string{}, f.args...), "-apply", "-verified-failed", "-offline-no-redis", "-backup", filepath.Join(filepath.Dir(f.path), backupName))
	return run(args, &bytes.Buffer{})
}

func TestSynlinkRecoveryPreviewAndIdempotentFunding(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		t.Run(fmt.Sprint("subscription=", subscription), func(t *testing.T) {
			f := newRecoveryFixture(t, subscription)
			before, err := os.ReadFile(f.path)
			require.NoError(t, err)
			var preview bytes.Buffer
			require.NoError(t, run(f.args, &preview))
			assert.Contains(t, preview.String(), "apply=false")
			assert.NotContains(t, preview.String(), "fixture-provider-task")
			after, err := os.ReadFile(f.path)
			require.NoError(t, err)
			assert.Equal(t, before, after, "preview must not write database bytes")
			require.NoError(t, f.apply(t, "backup-1.db"))
			require.NoError(t, f.apply(t, "backup-2.db"))
			var saved model.Task
			require.NoError(t, f.db.First(&saved, f.task.ID).Error)
			assert.EqualValues(t, model.TaskStatusFailure, saved.Status)
			assert.Equal(t, "100%", saved.Progress)
			assert.NotZero(t, saved.FinishTime)
			assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
			assert.Zero(t, saved.Quota)
			assert.Equal(t, f.task.Properties, saved.Properties)
			assert.Equal(t, f.task.PrivateData.Execution, saved.PrivateData.Execution)
			assert.Equal(t, f.task.PrivateData.UpstreamTaskID, saved.PrivateData.UpstreamTaskID)
			var user model.User
			require.NoError(t, f.db.First(&user, 2).Error)
			if subscription {
				assert.Equal(t, 900, user.Quota)
				var sub model.UserSubscription
				require.NoError(t, f.db.First(&sub, 6).Error)
				assert.Zero(t, sub.AmountUsed)
			} else {
				assert.Equal(t, 1000, user.Quota)
			}
			var token model.Token
			require.NoError(t, f.db.First(&token, 3).Error)
			assert.Equal(t, 1000, token.RemainQuota)
			assert.Zero(t, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, f.db.Where("type = ?", model.LogTypeRefund).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 100, logs[0].Quota)
			var audits []model.AuditLog
			require.NoError(t, f.db.Find(&audits).Error)
			require.Len(t, audits, 1)
			assert.Equal(t, 1, audits[0].UserId)
			assert.Equal(t, "task.synlink_verified_failure", audits[0].Action)
			assert.NotContains(t, audits[0].Content, "fixture-provider-task")
			backup, err := os.Stat(filepath.Join(filepath.Dir(f.path), "backup-1.db"))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0600), backup.Mode().Perm())
		})
	}
}

func TestSynlinkRecoveryRejectsUnverifiedAndChangedFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   string
		change func(*testing.T, *recoveryFixture)
	}{
		{"owner", "task_scope_mismatch", func(t *testing.T, f *recoveryFixture) { f.args = append(f.args, "-user-id", "99") }},
		{"application", "task_scope_mismatch", func(t *testing.T, f *recoveryFixture) { f.args = append(f.args, "-app-id", "0") }},
		{"channel", "task_scope_mismatch", func(t *testing.T, f *recoveryFixture) { f.args = append(f.args, "-channel-id", "9") }},
		{"operator", "operator_invalid", func(t *testing.T, f *recoveryFixture) { f.args = append(f.args, "-operator-id", "2") }},
		{"quota", "quota_mismatch", func(t *testing.T, f *recoveryFixture) { f.args = append(f.args, "-expected-quota", "101") }},
		{"unsafe evidence", "invalid_input", func(t *testing.T, f *recoveryFixture) {
			f.args = append(f.args, "-evidence-ref", "https://private.example")
		}},
		{"terminal won since preview", "task_state_conflict", func(t *testing.T, f *recoveryFixture) {
			require.NoError(t, run(f.args, &bytes.Buffer{}))
			require.NoError(t, f.db.Model(&f.task).Update("status", model.TaskStatusSuccess).Error)
		}},
		{"new version", "frozen_contract_mismatch", func(t *testing.T, f *recoveryFixture) {
			f.task.PrivateData.Execution.TaskPlugin.Version = "1.3.4"
			require.NoError(t, f.db.Model(&f.task).Update("private_data", f.task.PrivateData).Error)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecoveryFixture(t, false)
			tc.change(t, f)
			err := f.apply(t, "rejected.db")
			require.ErrorContains(t, err, "stage=preflight code="+tc.code)
			var audits int64
			require.NoError(t, f.db.Model(&model.AuditLog{}).Count(&audits).Error)
			assert.Zero(t, audits)
			var user model.User
			require.NoError(t, f.db.First(&user, 2).Error)
			assert.Equal(t, 900, user.Quota)
		})
	}
	f := newRecoveryFixture(t, false)
	require.Error(t, run(append(f.args, "-apply"), &bytes.Buffer{}))
	require.Error(t, run([]string{"-database", filepath.Join(t.TempDir(), "missing.db")}, &bytes.Buffer{}))
}

func TestSynlinkRecoveryAuditRollbackAndRefundResume(t *testing.T) {
	f := newRecoveryFixture(t, false)
	require.NoError(t, f.db.Exec(`CREATE TRIGGER reject_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'fixture-sensitive-diagnostic'); END`).Error)
	err := f.apply(t, "audit-failure.db")
	require.ErrorContains(t, err, "stage=terminal_decision code=database_error")
	assert.NotContains(t, err.Error(), "fixture-sensitive-diagnostic")
	var task model.Task
	require.NoError(t, f.db.First(&task, f.task.ID).Error)
	assert.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
	assert.Nil(t, task.PrivateData.AsyncBilling.TargetQuota)
	require.NoError(t, f.db.Exec("DROP TRIGGER reject_audit").Error)
	require.NoError(t, f.db.Exec(`CREATE TRIGGER reject_refund BEFORE UPDATE OF remain_quota ON tokens BEGIN SELECT RAISE(ABORT, 'fixture-sensitive-diagnostic'); END`).Error)
	err = f.apply(t, "refund-failure.db")
	require.ErrorContains(t, err, "refund pending")
	assert.Contains(t, err.Error(), "stage=refund code=database_error")
	assert.NotContains(t, err.Error(), "fixture-sensitive-diagnostic")
	require.NoError(t, f.db.First(&task, f.task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, task.Status)
	assert.Equal(t, 100, task.Quota)
	assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
	var user model.User
	require.NoError(t, f.db.First(&user, 2).Error)
	assert.Equal(t, 900, user.Quota, "wallet change rolls back when token update fails")
	require.NoError(t, f.db.Exec("DROP TRIGGER reject_refund").Error)
	require.NoError(t, f.apply(t, "resumed.db"))
	require.NoError(t, f.db.First(&user, 2).Error)
	assert.Equal(t, 1000, user.Quota)
	// Never overwrite an earlier backup, even on an otherwise idempotent repeat.
	require.ErrorContains(t, f.apply(t, "resumed.db"), "backup path must be new")
}

func TestSynlinkRecoveryResumesLogDeliveryWithoutRefundingAgain(t *testing.T) {
	f := newRecoveryFixture(t, false)
	require.NoError(t, f.db.Exec(`CREATE TRIGGER reject_log BEFORE INSERT ON logs BEGIN SELECT RAISE(ABORT, 'fixture-sensitive-diagnostic'); END`).Error)
	err := f.apply(t, "log-failure.db")
	require.ErrorContains(t, err, "log delivery pending")
	assert.Contains(t, err.Error(), "stage=log_delivery code=database_error")
	assert.NotContains(t, err.Error(), "fixture-sensitive-diagnostic")
	var task model.Task
	require.NoError(t, f.db.First(&task, f.task.ID).Error)
	assert.Equal(t, model.TaskBillingStateSettled, task.BillingState)
	assert.Zero(t, task.Quota)
	var user model.User
	require.NoError(t, f.db.First(&user, 2).Error)
	assert.Equal(t, 1000, user.Quota)
	// An unrelated evidence reference cannot adopt the prior manual decision.
	oldArgs := f.args
	f.args = append(append([]string{}, f.args...), "-evidence-ref", "different_evidence")
	require.ErrorContains(t, f.apply(t, "wrong-evidence.db"), "stage=preflight code=evidence_conflict")
	f.args = oldArgs
	require.NoError(t, f.db.Exec("DROP TRIGGER reject_log").Error)
	require.NoError(t, f.apply(t, "log-resume.db"))
	require.NoError(t, f.db.First(&user, 2).Error)
	assert.Equal(t, 1000, user.Quota)
	var logs []model.Log
	require.NoError(t, f.db.Where("type = ?", model.LogTypeRefund).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, 100, logs[0].Quota)
}

func TestSynlinkRecoveryDiagnosticsDistinguishDatabaseFromTaskConditions(t *testing.T) {
	f := newRecoveryFixture(t, false)
	require.NoError(t, f.db.Exec("ALTER TABLE users RENAME TO unavailable_users").Error)
	var output bytes.Buffer
	err := run(f.args, &output)
	require.ErrorContains(t, err, "stage=preflight code=database_error")
	assert.NotContains(t, err.Error(), "operator_invalid")
	assert.NotContains(t, err.Error(), "SELECT")
	assert.NotContains(t, err.Error(), f.path)
	assert.Empty(t, output.String())
	var saved model.Task
	require.NoError(t, f.db.First(&saved, f.task.ID).Error)
	assert.Equal(t, model.TaskStatusReconciliationRequired, saved.Status)
	assert.Equal(t, 100, saved.Quota)
}

func TestSynlinkRecoveryArgumentErrorsDoNotEchoPrivateValues(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"-operator-id", "fixture-private-argument"}, &output)
	require.ErrorContains(t, err, "stage=arguments code=invalid_input")
	assert.NotContains(t, err.Error(), "fixture-private-argument")
	assert.NotContains(t, output.String(), "fixture-private-argument")
}

func TestSynlinkRecoveryDiagnosticsNeverExposeWrappedDetails(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"known rejection", fmt.Errorf("fixture-private-detail: %w", model.ErrSynlinkRecoveryQuotaMismatch), "quota_mismatch"},
		{"funding", fmt.Errorf("fixture-private-detail: %w", model.ErrTaskBillingInsufficientFunding), "funding_conflict"},
		{"missing record", fmt.Errorf("fixture-private-detail: %w", gorm.ErrRecordNotFound), "frozen_record_missing"},
		{"unknown", errors.New("fixture-private-detail"), "operation_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := recoveryDiagnostic("refund", tc.err)
			require.ErrorContains(t, err, "stage=refund code="+tc.code)
			assert.NotContains(t, err.Error(), "fixture-private-detail")
		})
	}
}

func TestSynlinkRecoveryHelpRemainsAvailable(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, run([]string{"-help"}, &output))
	assert.Contains(t, output.String(), "-expected-quota")
	assert.Contains(t, output.String(), "-verified-failed")
}
