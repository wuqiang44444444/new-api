package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newImageTaskForTest(userID int, quota int) *Task {
	execution := &TaskImageExecutionData{
		Operation:      "generations",
		Prompt:         "a cat",
		N:              1,
		ChannelType:    24,
		ChannelBaseUrl: "https://gemini.example.com",
		ChannelKey:     "test-key",
		UpstreamModel:  "gemini-3.1-flash-image",
		FundsHeld:      true,
		HeldQuota:      quota,
	}
	task := &Task{
		TaskID:         GenerateTaskID(),
		Platform:       ImageTaskPlatform(24),
		UserId:         userID,
		AppID:          7,
		Group:          "default",
		ChannelId:      42,
		Action:         TaskActionImageGeneration,
		Status:         TaskStatusQueued,
		ClientProtocol: TaskClientProtocolImageOpenAIV1,
		SubmitTime:     common.GetTimestamp(),
		Progress:       "0%",
		Quota:          quota,
		PrivateData:    TaskPrivateData{ImageTask: execution, AppID: 7},
	}
	return task
}

func insertImageTaskForTest(t *testing.T, task *Task) {
	t.Helper()
	require.NoError(t, DB.Unscoped().Where("id = ?", task.UserId).Delete(&User{}).Error)
	require.NoError(t, DB.Create(&User{Id: task.UserId, Username: fmt.Sprintf("image-%d", task.UserId), Quota: 100000, AffCode: fmt.Sprintf("img%d", task.UserId)}).Error)
	require.NoError(t, InsertImageTask(ImageTaskInsertParams{
		Task:        task,
		GlobalScope: ImageTaskAdmissionScopeGlobal(),
		GlobalLimit: 0,
		AppScope:    ImageTaskAdmissionScopeApp(task.UserId, task.AppID),
		AppLimit:    0,
	}))
}

func TestImageTaskInsertIncrementsAdmissionAndBindsIdempotency(t *testing.T) {
	cleanupImageTaskFixtures(t)
	task := newImageTaskForTest(1001, 500)
	insertImageTaskForTest(t, task)

	var stored Task
	require.NoError(t, DB.Where("task_id = ?", task.TaskID).First(&stored).Error)
	assert.True(t, IsImageTask(&stored))
	require.NotNil(t, stored.PrivateData.ImageTask)
	assert.Equal(t, "gemini-3.1-flash-image", stored.PrivateData.ImageTask.UpstreamModel)

	slot := func(scope string) int {
		t.Helper()
		var row ImageTaskSlot
		require.NoError(t, DB.Where("scope = ?", scope).First(&row).Error)
		return row.Count
	}
	assert.Equal(t, 1, slot(ImageTaskAdmissionScopeGlobal()))
	assert.Equal(t, 1, slot(ImageTaskAdmissionScopeApp(1001, 7)))

	// 终态释放受理占用。
	_, err := FinishImageTaskFailure(task, TaskStatusFailure, "rejected")
	require.NoError(t, err)
	assert.Equal(t, 0, slot(ImageTaskAdmissionScopeGlobal()))
}

func TestImageTaskAdmissionLimitRejectsAndRollsBack(t *testing.T) {
	cleanupImageTaskFixtures(t)
	require.NoError(t, DB.Create(&ImageTaskSlot{
		Scope: ImageTaskAdmissionScopeApp(1002, 7), Count: 1, UpdatedAt: common.GetTimestamp(),
	}).Error)

	task := newImageTaskForTest(1002, 100)
	task.TaskID = GenerateTaskID()
	err := InsertImageTask(ImageTaskInsertParams{
		Task:        task,
		GlobalScope: ImageTaskAdmissionScopeGlobal(),
		GlobalLimit: 0,
		AppScope:    ImageTaskAdmissionScopeApp(1002, 7),
		AppLimit:    1,
	})
	require.Error(t, err)
	assert.True(t, IsImageSlotLimitError(err))
	assert.True(t, IsImageSlotAppLimit(err))

	// 回滚后任务不得存在。
	var count int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestImageTaskClaimCASAndExecutionSlots(t *testing.T) {
	cleanupImageTaskFixtures(t)
	task := newImageTaskForTest(1003, 300)
	insertImageTaskForTest(t, task)

	claimed, ok, err := ClaimImageTask(task.TaskID,
		ImageTaskExecutionScopeGlobal(), 5,
		ImageTaskExecutionScopeChannel(42), 5)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, TaskStatus("IN_PROGRESS"), claimed.Status)
	require.NotNil(t, claimed.PrivateData.ImageTask)
	assert.True(t, claimed.PrivateData.ImageTask.LeaseAt > 0)

	var execSlot ImageTaskSlot
	require.NoError(t, DB.Where("scope = ?", ImageTaskExecutionScopeGlobal()).First(&execSlot).Error)
	assert.Equal(t, 1, execSlot.Count)

	// 二次领取必须失败（CAS）。
	_, ok, err = ClaimImageTask(task.TaskID,
		ImageTaskExecutionScopeGlobal(), 5,
		ImageTaskExecutionScopeChannel(42), 5)
	require.NoError(t, err)
	assert.False(t, ok)

	// 发送许可 → 终态成功提交工件与用量。
	won, err := MarkImageTaskSending(claimed)
	require.NoError(t, err)
	assert.True(t, won)

	artifacts := []TaskImageArtifact{{ObjectKey: "images/tasks/x/result-0", MimeType: "image/png", Size: 3}}
	won, err = FinishImageTaskSuccess(claimed, artifacts, &dto.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30})
	require.NoError(t, err)
	assert.True(t, won)

	var stored Task
	require.NoError(t, DB.Where("task_id = ?", task.TaskID).First(&stored).Error)
	assert.Equal(t, TaskStatus("SUCCESS"), stored.Status)
	assert.Equal(t, "100%", stored.Progress)
	require.Len(t, stored.PrivateData.ImageTask.Artifacts, 1)
	assert.Equal(t, 30, stored.PrivateData.ImageTask.Usage.TotalTokens)
}

func TestImageTaskUnfundedAndExpiryRecoveryQueries(t *testing.T) {
	cleanupImageTaskFixtures(t)

	expired := newImageTaskForTest(1005, 200)
	expired.PrivateData.ImageTask.QueueDeadlineAt = common.GetTimestamp() - 5
	insertImageTaskForTest(t, expired)

	pending := newImageTaskForTest(1006, 200)
	pending.PrivateData.ImageTask.QueueDeadlineAt = common.GetTimestamp() + 600
	insertImageTaskForTest(t, pending)

	expiredTasks := GetExpiredQueuedImageTasks(common.GetTimestamp(), 10)
	require.Len(t, expiredTasks, 1)
	assert.Equal(t, expired.TaskID, expiredTasks[0].TaskID)

	queued := GetQueuedImageTasks(10)
	assert.Len(t, queued, 2)
}

func TestImageTaskSweepExclusionFromVideoPolling(t *testing.T) {
	cleanupImageTaskFixtures(t)
	insertImageTaskForTest(t, newImageTaskForTest(1007, 100))
	// 历史任务 client_protocol 为 NULL 时必须继续被视频轮询扫描收录。
	legacy := newImageTaskForTest(1008, 0)
	legacy.ClientProtocol = ""
	legacy.TaskID = GenerateTaskID()
	require.NoError(t, legacy.Insert())

	tasks := GetAllUnFinishSyncTasks(10)
	foundImage, foundLegacy := false, false
	for _, task := range tasks {
		if task.TaskID == legacy.TaskID {
			foundLegacy = true
		}
		if IsImageTask(task) {
			foundImage = true
		}
	}
	assert.True(t, foundLegacy, "legacy NULL-protocol tasks must stay in the video polling feed")
	assert.False(t, foundImage, "explicit image tasks must be excluded from the video polling feed")

	timedOut := GetTimedOutUnfinishedTasks(common.GetTimestamp()+600, 10)
	for _, task := range timedOut {
		assert.False(t, IsImageTask(task), "image tasks must be excluded from the generic timeout sweep")
	}
}

func TestRebuildImageTaskSlotsFromFacts(t *testing.T) {
	cleanupImageTaskFixtures(t)
	task := newImageTaskForTest(1009, 100)
	insertImageTaskForTest(t, task)

	// 人为漂移计数，重建后必须与 Task 事实一致。
	require.NoError(t, DB.Model(&ImageTaskSlot{}).Where("scope = ?", ImageTaskAdmissionScopeGlobal()).
		Update("count", 999).Error)
	require.NoError(t, RebuildImageTaskSlots())

	var rebuilt ImageTaskSlot
	require.NoError(t, DB.Where("scope = ?", ImageTaskAdmissionScopeGlobal()).First(&rebuilt).Error)
	assert.Equal(t, 1, rebuilt.Count)
}

func cleanupImageTaskFixtures(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Where("client_protocol = ?", TaskClientProtocolImageOpenAIV1).Delete(&Task{}).Error)
	require.NoError(t, DB.Exec("DELETE FROM image_task_slots").Error)
	// 只清掉本文件产生的 NULL-protocol 测试行（按用户 ID 前缀约定）。
	require.NoError(t, DB.Where("user_id IN ?", []int{1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008, 1009, 1010}).Delete(&Task{}).Error)
}

// 评审 S6：逐图登记幂等且 FindImageTaskArtifact 只解析已登记事实。
func TestAppendAndFindImageTaskArtifact(t *testing.T) {
	cleanupImageTaskFixtures(t)
	task := newImageTaskForTest(1010, 100)
	insertImageTaskForTest(t, task)

	artifact := TaskImageArtifact{
		ObjectKey: ImageTaskArtifactObjectKey(task.TaskID, "result-0"),
		MimeType:  "image/png", Size: 3,
	}
	won, err := AppendImageTaskArtifact(task, artifact)
	require.NoError(t, err)
	require.True(t, won)
	won, err = AppendImageTaskArtifact(task, artifact)
	require.NoError(t, err)
	require.True(t, won)

	var stored Task
	require.NoError(t, DB.Where("task_id = ?", task.TaskID).First(&stored).Error)
	require.Len(t, stored.PrivateData.ImageTask.Artifacts, 1)

	found := FindImageTaskArtifact(&stored, "result-0")
	require.NotNil(t, found)
	assert.Equal(t, artifact.ObjectKey, found.ObjectKey)
	assert.Nil(t, FindImageTaskArtifact(&stored, "result-9"))
}

// 评审 S9：重建必须把已消失 scope 的残留计数归零。
func TestRebuildImageTaskSlotsZeroesStaleScopes(t *testing.T) {
	cleanupImageTaskFixtures(t)
	require.NoError(t, DB.Create(&ImageTaskSlot{
		Scope: ImageTaskAdmissionScopeApp(4242, 7), Count: 5, UpdatedAt: common.GetTimestamp(),
	}).Error)
	require.NoError(t, RebuildImageTaskSlots())

	var stale ImageTaskSlot
	require.NoError(t, DB.Where("scope = ?", ImageTaskAdmissionScopeApp(4242, 7)).First(&stale).Error)
	assert.Equal(t, 0, stale.Count)
}

// 评审 S7：待核实/未终态图片任务的幂等 claim 到期后不得被重置。
func TestImageIdempotencyRetentionWhileTaskActive(t *testing.T) {
	cleanupImageTaskFixtures(t)
	userID := 1010
	task := newImageTaskForTest(userID, 100)
	insertImageTaskForTest(t, task)

	claim := &TaskCreateIdempotency{
		UserID: userID, Protocol: TaskClientProtocolImageOpenAIV1,
		KeyHash: "retention-test", RequestHash: "hash-a",
		Status: TaskCreateIdempotencyComplete, TaskID: task.TaskID,
		ExpiresAt: 1, CreatedAt: 1, UpdatedAt: 1,
	}
	require.NoError(t, DB.Create(claim).Error)

	// 任务未终态：同 key 同请求不得重置，返回既有 claim（重放语义）。
	existing, created, err := ClaimTaskCreateIdempotency(userID, TaskClientProtocolImageOpenAIV1, "retention-test", "hash-a", 1)
	require.NoError(t, err)
	assert.False(t, created, "active image task must retain its idempotency binding")
	assert.Equal(t, task.TaskID, existing.TaskID)

	// 任务终态后允许到期重置。
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Update("status", TaskStatusFailure).Error)
	_, created, err = ClaimTaskCreateIdempotency(userID, TaskClientProtocolImageOpenAIV1, "retention-test", "hash-a", 1)
	require.NoError(t, err)
	assert.True(t, created, "terminal image task may release the expired claim")
}

// 评审 S6：可信 Provider 任务 ID 的待核实任务可被恢复查询入口发现。
func TestGetReconcilableImageTasksRequiresProviderID(t *testing.T) {
	cleanupImageTaskFixtures(t)
	withID := newImageTaskForTest(1010, 100)
	withID.Status = TaskStatusReconciliationRequired
	withID.PrivateData.ImageTask.ProviderTaskID = "fc-123"
	require.NoError(t, DB.Create(withID).Error)

	withoutID := newImageTaskForTest(1010, 100)
	withoutID.Status = TaskStatusReconciliationRequired
	withoutID.TaskID = GenerateTaskID()
	require.NoError(t, DB.Create(withoutID).Error)

	tasks := GetReconcilableImageTasks(10)
	require.Len(t, tasks, 1)
	assert.Equal(t, withID.TaskID, tasks[0].TaskID)
}
