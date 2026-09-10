package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSeedancePluginUsageTest(t *testing.T) {
	t.Helper()
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	var err error
	DB, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, DB.AutoMigrate(&Task{}, &TaskCreateAttempt{}, &TaskPlugin{}))
}

func seedancePinnedTask(taskID, status, billingState, pluginVersion, platform string) *Task {
	task := &Task{
		TaskID:   taskID,
		Platform: constant.TaskPlatform(platform),
		Status:   TaskStatus(status),
	}
	task.PrivateData.Execution = &TaskExecutionSnapshot{
		TaskPlugin: &TaskPluginSnapshot{Key: "seedance-link", Version: pluginVersion},
	}
	if billingState != "" {
		task.BillingState = TaskBillingState(billingState)
	}
	return task
}

func TestGetSeedancePluginExecutionUsageCountsExecutionDependencies(t *testing.T) {
	setupSeedancePluginUsageTest(t)

	fixtures := []*Task{
		seedancePinnedTask("task-in-progress", "IN_PROGRESS", "", "1.0.0", "62"),
		seedancePinnedTask("task-reconciliation", "RECONCILIATION_REQUIRED", "", "1.0.0", "62"),
		seedancePinnedTask("task-awaiting-usage", "SUCCESS", "awaiting_usage", "1.0.0", "62"),
		seedancePinnedTask("task-unsettled-pending", "SUCCESS", "pending", "1.0.0", "62"),
		seedancePinnedTask("task-failed-refund-pending", "FAILURE", "failed", "1.0.0", "62"),
		seedancePinnedTask("task-debt", "SUCCESS", "debt", "1.0.0", "62"),
		// Still counted: visible SUCCESS tasks refresh on ModelArk GET.
		seedancePinnedTask("task-settled", "SUCCESS", "settled", "1.0.0", "62"),
		// Not counted: pinned to another version.
		seedancePinnedTask("task-other-version", "IN_PROGRESS", "", "2.0.0", "62"),
		// Not counted: historical task without a plugin snapshot.
		{TaskID: "task-legacy", Platform: constant.TaskPlatform("62"), Status: TaskStatusInProgress},
		// Not counted: other platform with the same pin shape.
		seedancePinnedTask("task-other-platform", "IN_PROGRESS", "", "1.0.0", "54"),
	}
	for _, task := range fixtures {
		require.NoError(t, DB.Create(task).Error)
	}

	attemptSnapshot := func(pluginVersion string) []byte {
		task := seedancePinnedTask("attempt-template", "IN_PROGRESS", "", pluginVersion, "62")
		snapshot, err := common.Marshal(taskAttemptRecoverySnapshot{Task: *task, PrivateData: task.PrivateData})
		require.NoError(t, err)
		return snapshot
	}
	attempts := []TaskCreateAttempt{
		{AttemptID: "a1", UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptPrepared, RecoverySnapshot: attemptSnapshot("1.0.0")},
		{AttemptID: "a2", UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptSending, RecoverySnapshot: attemptSnapshot("1.0.0")},
		{AttemptID: "a3", UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptUnknown, RecoverySnapshot: attemptSnapshot("1.0.0")},
		{AttemptID: "a4", UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptUpstreamSucceeded, RecoverySnapshot: attemptSnapshot("1.0.0")},
		// Not counted: completed attempt.
		{AttemptID: "a5", UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptComplete, RecoverySnapshot: attemptSnapshot("1.0.0")},
		// Counted: frozen plugin identity survives protocol registration changes.
		{AttemptID: "a6", UpstreamProtocol: "modelark_v3_volcengine", Status: TaskCreateAttemptSending, RecoverySnapshot: attemptSnapshot("1.0.0")},
		// Not counted: pinned to another version.
		{AttemptID: "a7", UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptSending, RecoverySnapshot: attemptSnapshot("2.0.0")},
	}
	for i := range attempts {
		require.NoError(t, DB.Create(&attempts[i]).Error)
	}

	tasks, attemptRefs, err := GetSeedancePluginExecutionUsage("seedance-link", "1.0.0")
	require.NoError(t, err)

	taskIDs := make([]string, 0, len(tasks))
	for _, ref := range tasks {
		taskIDs = append(taskIDs, ref.TaskId)
	}
	assert.ElementsMatch(t, []string{
		"task-in-progress", "task-reconciliation", "task-awaiting-usage",
		"task-unsettled-pending", "task-failed-refund-pending", "task-debt", "task-settled",
	}, taskIDs)

	attemptIDs := make([]string, 0, len(attemptRefs))
	for _, ref := range attemptRefs {
		attemptIDs = append(attemptIDs, ref.Status)
	}
	assert.ElementsMatch(t, []string{"prepared", "sending", "unknown", "upstream_succeeded", "sending"}, attemptIDs)
}

// TestGetSeedancePluginExecutionUsageCountsPreparedFrozenPin verifies the
// race closure: the plugin identity is persisted in the prepared attempt's
// frozen connection (the earliest durable record of a creation), so a
// concurrent version deletion is blocked before the provider POST even
// though the recovery template has not been staged yet.
func TestGetSeedancePluginExecutionUsageCountsPreparedFrozenPin(t *testing.T) {
	setupSeedancePluginUsageTest(t)

	frozen, err := common.Marshal(map[string]string{
		"plugin_key": "seedance-link", "plugin_version": "1.0.0",
	})
	require.NoError(t, err)
	attempt := TaskCreateAttempt{
		AttemptID: "prepared-1", UpstreamProtocol: "feicai_videos_v1",
		Status: TaskCreateAttemptPrepared, FrozenConnectionSnapshot: frozen,
	}
	require.NoError(t, DB.Create(&attempt).Error)

	tasks, attempts, err := GetSeedancePluginExecutionUsage("seedance-link", "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, tasks)
	require.Len(t, attempts, 1)
	assert.Equal(t, "prepared", attempts[0].Status)

	// A prepared attempt without any pin is not a dependency.
	plain := TaskCreateAttempt{
		AttemptID: "prepared-2", UpstreamProtocol: "feicai_videos_v1",
		Status: TaskCreateAttemptPrepared,
	}
	require.NoError(t, DB.Create(&plain).Error)
	_, attempts, err = GetSeedancePluginExecutionUsage("seedance-link", "1.0.0")
	require.NoError(t, err)
	assert.Len(t, attempts, 1)

	// Empty version counts every version (list views).
	other := TaskCreateAttempt{
		AttemptID: "prepared-3", UpstreamProtocol: "feicai_videos_v1",
		Status: TaskCreateAttemptSending,
	}
	otherFrozen, err := common.Marshal(map[string]string{
		"plugin_key": "seedance-link", "plugin_version": "2.0.0",
	})
	require.NoError(t, err)
	other.FrozenConnectionSnapshot = otherFrozen
	require.NoError(t, DB.Create(&other).Error)
	_, attempts, err = GetSeedancePluginExecutionUsage("seedance-link", "")
	require.NoError(t, err)
	assert.Len(t, attempts, 2)
}

func TestGetSeedancePluginExecutionUsageRequiresInputs(t *testing.T) {
	setupSeedancePluginUsageTest(t)
	tasks, attempts, err := GetSeedancePluginExecutionUsage("", "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, tasks)
	assert.Empty(t, attempts)

	tasks, attempts, err = GetSeedancePluginExecutionUsage("seedance-link", "")
	require.NoError(t, err)
	assert.Empty(t, tasks)
	assert.Empty(t, attempts)

	assert.True(t, SeedancePluginExecutionUsageApplies("seedance-link"))
	assert.False(t, SeedancePluginExecutionUsageApplies("doubao"))
	assert.False(t, SeedancePluginExecutionUsageApplies(""))
}
