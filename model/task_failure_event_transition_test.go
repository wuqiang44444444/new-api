package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkRecoveryEmitsOnlyForCommittedTransition(t *testing.T) {
	db := setupErrorEventTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Task{}, &AuditLog{}))
	require.NoError(t, db.Create(&User{Id: 1, Username: "root-fixture", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	task := Task{TaskID: "task_fixture", UserId: 2, AppID: 3, ChannelId: 4, Quota: 100, Status: TaskStatusReconciliationRequired, BillingState: TaskBillingStatePending, FailReason: "untrusted Synlink task identity",
		PrivateData: TaskPrivateData{BillingSource: "wallet", UpstreamTaskID: "upstream-fixture", VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1,
			Execution: &TaskExecutionSnapshot{TaskPlugin: &TaskPluginSnapshot{Key: "seedance-link", Version: "1.3.3", APIVersion: 3}}, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStatePending}}}
	require.NoError(t, db.Create(&task).Error)
	events := make(chan clienterrlog.Event, 4)
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error { events <- e; return nil })
	t.Cleanup(func() { clienterrlog.SetEventPersister(persistErrorEvent) })
	before := clienterrlog.CurrentHealth().Accepted
	scope := SynlinkFailureRecovery{TaskID: task.TaskID, UserID: 2, AppID: 3, ChannelID: 4, OperatorID: 1, ExpectedQuota: 100, EvidenceRef: "verified_fixture"}
	_, err := RecoverSynlinkFailedTask(scope, false)
	require.NoError(t, err)
	assert.Equal(t, before, clienterrlog.CurrentHealth().Accepted)
	_, err = RecoverSynlinkFailedTask(scope, true)
	require.NoError(t, err)
	select {
	case event := <-events:
		assert.Equal(t, "manual_verification", event.Stage)
	case <-time.After(3 * time.Second):
		t.Fatal("missing committed event")
	}
	for _, apply := range []bool{true, false} {
		_, err := RecoverSynlinkFailedTask(scope, apply)
		require.NoError(t, err)
	}
	assert.Equal(t, before+1, clienterrlog.CurrentHealth().Accepted)
	var saved Task
	require.NoError(t, db.First(&saved, task.ID).Error)
	assert.Equal(t, 100, saved.Quota, "recovery only requests refund; the billing owner settles it")
}

func TestBatchFailureEventUsesCommittedReasonAndDoesNotRepeat(t *testing.T) {
	db := setupErrorEventTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Task{}, &BatchJob{}, &Log{}, &QuotaData{}))
	require.NoError(t, db.Create(&User{Id: 1, Username: "batch-fixture"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 2}).Error)
	task := Task{TaskID: "batch-fixture", UserId: 1, ChannelId: 2, Status: TaskStatusInProgress, FailReason: ""}
	require.NoError(t, db.Create(&task).Error)
	job := BatchJob{Id: "batch-fixture", UserId: 1, PublicStatus: "failed", SanitizeError: "Batch processing failed", SettleState: BatchSettlePending}
	require.NoError(t, db.Create(&job).Error)
	events := make(chan clienterrlog.Event, 4)
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error { events <- e; return nil })
	t.Cleanup(func() { clienterrlog.SetEventPersister(persistErrorEvent) })
	before := clienterrlog.CurrentHealth().Accepted
	require.NoError(t, CompleteBatchSettlement(&job, &task, 0, &LogOther{}, 0, 0))
	var event clienterrlog.Event
	select {
	case event = <-events:
	case <-time.After(3 * time.Second):
		t.Fatal("missing batch failure event")
	}
	assert.NotEmpty(t, event.Detail["fail_reason"])
	var saved Task
	require.NoError(t, db.First(&saved, task.ID).Error)
	assert.Equal(t, "Batch processing failed", saved.FailReason)
	assert.Equal(t, "Batch_processing_failed", event.Detail["fail_reason"])
	assert.Empty(t, task.FailReason, "do not mutate the caller's snapshot")
	require.NoError(t, CompleteBatchSettlement(&job, &task, 0, &LogOther{}, 0, 0))
	assert.Equal(t, before+1, clienterrlog.CurrentHealth().Accepted)
}
