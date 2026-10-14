package model

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func videoResultTestTask(taskID string, platform int) *Task {
	data, _ := common.Marshal(map[string]any{
		"content": map[string]any{"last_frame_url": "https://provider.example/frame.jpg"},
	})
	return &Task{
		TaskID:         taskID,
		Platform:       constant.TaskPlatform(strconv.Itoa(platform)),
		UserId:         71,
		AppID:          900,
		ClientProtocol: TaskClientProtocolModelArkV3,
		Status:         TaskStatusSuccess,
		FinishTime:     common.GetTimestamp(),
		Data:           data,
	}
}

func insertVideoResultTaskForTest(t *testing.T, task *Task) {
	t.Helper()
	require.NoError(t, DB.Create(task).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&TaskVideoResultObject{}, "task_id = ?", task.TaskID).Error
		_ = DB.Unscoped().Delete(&Task{}, "task_id = ?", task.TaskID).Error
	})
}

func TestVideoTaskResultPartsDetectLastFrame(t *testing.T) {
	plain := videoResultTestTask("task_parts_plain", constant.ChannelTypeSeedanceLink)
	plain.Data = []byte(`{"content":{"video_url":"https://provider.example/v.mp4"}}`)
	assert.Equal(t, []string{TaskVideoResultPartVideo}, VideoTaskResultParts(plain))

	withFrame := videoResultTestTask("task_parts_frame", constant.ChannelTypeSeedanceLink)
	assert.Equal(t, []string{TaskVideoResultPartVideo, TaskVideoResultPartLastFrame}, VideoTaskResultParts(withFrame))
}

func TestRegisterVideoResultObjectsIsIdempotent(t *testing.T) {
	task := videoResultTestTask("task_register_dup", constant.ChannelTypeSeedanceLink)
	insertVideoResultTaskForTest(t, task)

	require.NoError(t, RegisterVideoResultObjects(task, VideoTaskResultParts(task)))
	require.NoError(t, RegisterVideoResultObjects(task, VideoTaskResultParts(task)))

	rows, err := ListVideoResultObjects(task.TaskID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	parts := map[string]string{}
	for _, row := range rows {
		parts[row.Part] = row.Status
		assert.Equal(t, task.UserId, row.UserID)
		assert.Equal(t, task.AppID, row.AppID)
		assert.Equal(t, TaskVideoResultStatusPending, row.Status)
	}
	assert.Equal(t, map[string]string{
		TaskVideoResultPartVideo:     TaskVideoResultStatusPending,
		TaskVideoResultPartLastFrame: TaskVideoResultStatusPending,
	}, parts)
}

func TestClaimVideoResultObjectsCasAndStaleRecovery(t *testing.T) {
	task := videoResultTestTask("task_claim_cas", constant.ChannelTypeSeedanceLink)
	insertVideoResultTaskForTest(t, task)
	require.NoError(t, RegisterVideoResultObjects(task, []string{TaskVideoResultPartVideo}))

	now := common.GetTimestamp()
	first, err := ClaimVideoResultObjects(1, "runner-a", now, 1800)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "runner-a", first[0].ClaimedBy)
	assert.Equal(t, 1, first[0].Attempts)

	// 租约内不能被第二个 runner 领取。
	second, err := ClaimVideoResultObjects(1, "runner-b", now, 1800)
	require.NoError(t, err)
	assert.Empty(t, second)

	// 租约过期后可以被重新领取，且尝试次数递增。
	third, err := ClaimVideoResultObjects(1, "runner-b", now+1801, 1800)
	require.NoError(t, err)
	require.Len(t, third, 1)
	assert.Equal(t, "runner-b", third[0].ClaimedBy)
	assert.Equal(t, 2, third[0].Attempts)

	// 退避中的行不可领取。
	won, err := RescheduleVideoResultObjectRetry(third[0].ID, "runner-b", now+9999, "backoff", now+1801)
	require.NoError(t, err)
	require.True(t, won)
	deferred, err := ClaimVideoResultObjects(1, "runner-c", now+2000, 1800)
	require.NoError(t, err)
	assert.Empty(t, deferred)
}

func TestFinishVideoResultObjectTransitions(t *testing.T) {
	task := videoResultTestTask("task_finish_states", constant.ChannelTypeSeedanceLink)
	insertVideoResultTaskForTest(t, task)
	require.NoError(t, RegisterVideoResultObjects(task, []string{TaskVideoResultPartVideo}))
	now := common.GetTimestamp()
	claimed, err := ClaimVideoResultObjects(1, "runner", now, 1800)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	row := claimed[0]

	location := VideoResultStorageLocation{Backend: "s3", Endpoint: "http://oss.test", Bucket: "videos", Prefix: ""}
	won, err := FinishVideoResultObjectReady(row.ID, "other-runner", location, "videos/tasks/x/video", "video/mp4", 123, now)
	require.NoError(t, err)
	assert.False(t, won, "non-owner completion must be rejected")

	won, err = FinishVideoResultObjectReady(row.ID, "runner", location, "videos/tasks/task_finish_states/video", "video/mp4", 123, now)
	require.NoError(t, err)
	require.True(t, won)
	rows, listErr := ListVideoResultObjects(task.TaskID)
	require.NoError(t, listErr)
	stored := VideoResultReadyObject(rows, TaskVideoResultPartVideo)
	require.NotNil(t, stored)
	assert.Equal(t, location, stored.Location)
	assert.Equal(t, int64(123), stored.SizeBytes)

	// 已就绪的行不再是可领取工作。
	remaining, err := ClaimVideoResultObjects(5, "runner", now+1, 1800)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestFinishVideoResultObjectUnavailableAndLookup(t *testing.T) {
	task := videoResultTestTask("task_unavailable", constant.ChannelTypeMiniMaxLink)
	insertVideoResultTaskForTest(t, task)
	require.NoError(t, RegisterVideoResultObjects(task, []string{TaskVideoResultPartVideo}))
	now := common.GetTimestamp()
	claimed, err := ClaimVideoResultObjects(1, "runner", now, 1800)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	won, err := FinishVideoResultObjectUnavailable(claimed[0].ID, "runner", "upstream result expired", now+1)
	require.NoError(t, err)
	require.True(t, won)

	rows, err := ListVideoResultObjects(task.TaskID)
	require.NoError(t, err)
	stored := VideoResultReadyObject(rows, TaskVideoResultPartVideo)
	assert.Nil(t, stored, "unavailable rows must never be presigned")
	for i := range rows {
		if rows[i].Part == TaskVideoResultPartVideo {
			assert.Equal(t, TaskVideoResultStatusUnavailable, rows[i].Status)
			assert.Equal(t, "upstream result expired", rows[i].LastError)
			// 归属事实随行保存：交付路径必须先验证任务归属再按 task_id 取行。
			assert.Equal(t, task.UserId, rows[i].UserID)
			assert.Equal(t, task.AppID, rows[i].AppID)
		}
	}
}

func TestFindVideoResultRegistrationGaps(t *testing.T) {
	platforms := []constant.TaskPlatform{constant.TaskPlatform("67"), constant.TaskPlatform("69")}
	task := videoResultTestTask("task_gap", 67)
	insertVideoResultTaskForTest(t, task)

	gaps, err := FindVideoResultRegistrationGaps(platforms, common.GetTimestamp()-60, 10)
	require.NoError(t, err)
	found := false
	for _, gap := range gaps {
		if gap.TaskID == task.TaskID {
			found = true
		}
	}
	assert.True(t, found, "successful in-scope task without rows is a registration gap")

	require.NoError(t, RegisterVideoResultObjects(task, []string{TaskVideoResultPartVideo}))
	gaps, err = FindVideoResultRegistrationGaps(platforms, common.GetTimestamp()-60, 10)
	require.NoError(t, err)
	assert.Contains(t, gaps, *task, "missing last frame must remain recoverable")
	require.NoError(t, RegisterVideoResultObjects(task, VideoTaskResultParts(task)))
	gaps, err = FindVideoResultRegistrationGaps(platforms, common.GetTimestamp()-60, 10)
	require.NoError(t, err)
	for _, gap := range gaps {
		assert.NotEqual(t, task.TaskID, gap.TaskID)
	}

	// 窗口外的存量任务不是自动迁移对象。
	old := videoResultTestTask("task_gap_old", 67)
	old.CreatedAt = common.GetTimestamp() - 99999
	old.FinishTime = old.CreatedAt + 10
	insertVideoResultTaskForTest(t, old)
	gaps, err = FindVideoResultRegistrationGaps(platforms, common.GetTimestamp()-3600, 10)
	require.NoError(t, err)
	for _, gap := range gaps {
		assert.NotEqual(t, old.TaskID, gap.TaskID)
	}
}

func TestVideoResultRegistrationIsAtomicOnPartFailure(t *testing.T) {
	task := videoResultTestTask("task_atomic_parts", constant.ChannelTypeSeedanceLink)
	insertVideoResultTaskForTest(t, task)
	callback := "video_result_test:fail_frame"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		switch rows := tx.Statement.Dest.(type) {
		case *TaskVideoResultObject:
			if rows.Part == TaskVideoResultPartLastFrame {
				tx.AddError(errors.New("frame registration failed"))
			}
		case *[]TaskVideoResultObject:
			for _, row := range *rows {
				if row.Part == TaskVideoResultPartLastFrame {
					tx.AddError(errors.New("frame registration failed"))
					break
				}
			}
		}
	}))
	t.Cleanup(func() { DB.Callback().Create().Remove(callback) })
	require.Error(t, RegisterVideoResultObjects(task, VideoTaskResultParts(task)))
	rows, err := ListVideoResultObjects(task.TaskID)
	require.NoError(t, err)
	assert.Empty(t, rows, "failed frame registration must roll back the video row too")
}

func TestVideoResultRecoveryWindowUsesCompletionTime(t *testing.T) {
	task := videoResultTestTask("task_old_creation_recent_completion", constant.ChannelTypeSeedanceLink)
	task.CreatedAt = common.GetTimestamp() - 48*3600
	insertVideoResultTaskForTest(t, task)
	gaps, err := FindVideoResultRegistrationGaps([]constant.TaskPlatform{task.Platform}, common.GetTimestamp()-3600, 200)
	require.NoError(t, err)
	assert.Contains(t, gaps, *task, "a recently completed task must remain recoverable regardless of creation time")
}

func TestVideoResultRecoveryScansPastCompleteTasks(t *testing.T) {
	now := common.GetTimestamp()
	tasks := make([]Task, 201)
	objects := make([]TaskVideoResultObject, 201)
	ids := make([]string, 201)
	for i := range tasks {
		ids[i] = fmt.Sprintf("task_complete_page_%d", i)
		tasks[i] = *videoResultTestTask(ids[i], constant.ChannelTypeSeedanceLink)
		tasks[i].Data = nil // 没有尾帧的已完成任务不得阻塞后面的真实缺口。
		objects[i] = TaskVideoResultObject{TaskID: ids[i], Part: TaskVideoResultPartVideo, UserID: 71, AppID: 900, Status: TaskVideoResultStatusReady, CreatedAt: now, UpdatedAt: now}
	}
	require.NoError(t, DB.Create(&tasks).Error)
	require.NoError(t, DB.Create(&objects).Error)
	t.Cleanup(func() {
		DB.Where("task_id IN ?", ids).Delete(&TaskVideoResultObject{})
		DB.Unscoped().Where("task_id IN ?", ids).Delete(&Task{})
	})
	missing := videoResultTestTask("task_after_complete_page", constant.ChannelTypeSeedanceLink)
	insertVideoResultTaskForTest(t, missing)
	require.NoError(t, RegisterVideoResultObjects(missing, []string{TaskVideoResultPartVideo}))
	gaps, err := FindVideoResultRegistrationGaps([]constant.TaskPlatform{missing.Platform}, now-1, 1)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	assert.Equal(t, missing.TaskID, gaps[0].TaskID)
}
