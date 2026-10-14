package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEvidenceCleanupEnv(t *testing.T) (service.TaskRequestEvidenceObjectStore, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cleanup-test.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.TaskRequestEvidence{},
		&model.TaskRequestEvidenceEvent{},
		&model.TaskRequestEvidenceAccessLog{}, &model.Task{},
	))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	storeDir := t.TempDir()
	config := system_setting.TaskRequestEvidenceConfig{
		Enabled:             true,
		StorageDir:          storeDir,
		EncryptionKeyHex:    "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		MaxBodyBytes:        1 << 20,
		MaxResponseBytes:    1 << 20,
		WriteTimeoutSeconds: 5,
		RetentionDays:       7,
	}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		require.NoError(t, service.InitTaskRequestEvidenceStore(system_setting.TaskRequestEvidenceConfig{Enabled: false}))
		system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	})
	store := service.GetTaskRequestEvidenceStore()
	require.NotNil(t, store)
	return store, storeDir
}

func createCleanupTask(t *testing.T) (*model.SystemTask, string) {
	t.Helper()
	task, err := model.CreateSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention, nil, nil)
	require.NoError(t, err)
	_, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeTaskRequestEvidenceRetention, "runner-cleanup-1", common.GetTimestamp()+3600)
	require.NoError(t, err)
	require.True(t, claimed)
	return task, "runner-cleanup-1"
}

// seedExpiredEvidence creates an index older than the retention window with one
// committed event object and one reserved-but-uncommitted event (empty
// object_key) whose object exists on disk.
func seedExpiredEvidence(t *testing.T, store service.TaskRequestEvidenceObjectStore, storeDir string, createdAgo int64) *model.TaskRequestEvidence {
	t.Helper()
	now := common.GetTimestamp()
	evidence := &model.TaskRequestEvidence{
		RequestID:   "req-cleanup-" + strconv.Itoa(int(now)) + "-" + strconv.Itoa(int(createdAgo)),
		Kind:        "image_error",
		CreatedAt:   now - createdAgo,
		UpdatedAt:   now - createdAgo,
		RetainUntil: 0,
	}
	require.NoError(t, model.CreateTaskRequestEvidence(evidence))
	require.NoError(t, store.Put(service.EvidenceObjectKey(evidence.Id, 1), []byte("committed-body")))
	require.NoError(t, store.Put(service.EvidenceObjectKey(evidence.Id, 2), []byte("late-object")))
	events := []*model.TaskRequestEvidenceEvent{
		{EvidenceId: evidence.Id, Seq: 1, Stage: "north_receive", Phase: model.TaskRequestEvidencePhaseCompleted, ObjectKey: service.EvidenceObjectKey(evidence.Id, 1), StoredBytes: 14},
		{EvidenceId: evidence.Id, Seq: 2, Stage: "upstream_response", Phase: model.TaskRequestEvidencePhaseUnavailable, ObjectKey: ""},
	}
	for _, event := range events {
		require.NoError(t, model.DB.Create(event).Error)
	}
	return evidence
}

func TestEvidenceRetentionCleanupExpiresZeroRetentionRows(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	_, _ = store, storeDir
	task, runnerID := createCleanupTask(t)
	now := common.GetTimestamp()
	stale := seedExpiredEvidence(t, store, storeDir, 8*86400)
	fresh := seedExpiredEvidence(t, store, storeDir, 86400)

	evidenceRetentionHandler{}.Run(context.Background(), task, runnerID)

	var finished model.SystemTask
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&finished).Error)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)

	var reloaded model.TaskRequestEvidence
	assert.ErrorIs(t, model.DB.Where("id = ?", stale.Id).First(&reloaded).Error, gorm.ErrRecordNotFound)
	var remaining int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Where("evidence_id = ?", stale.Id).Count(&remaining).Error)
	assert.Zero(t, remaining)
	_, statErr := os.Stat(filepath.Join(storeDir, strconv.FormatInt(stale.Id, 10), "1.bin"))
	assert.True(t, os.IsNotExist(statErr))
	_, lateStatErr := os.Stat(filepath.Join(storeDir, strconv.FormatInt(stale.Id, 10), "2.bin"))
	assert.True(t, os.IsNotExist(lateStatErr))
	_, dirStatErr := os.Stat(filepath.Join(storeDir, strconv.FormatInt(stale.Id, 10)))
	assert.True(t, os.IsNotExist(dirStatErr))

	var kept model.TaskRequestEvidence
	require.NoError(t, model.DB.Where("id = ?", fresh.Id).First(&kept).Error)
	assert.False(t, kept.BodyExpired)
	_, freshStatErr := os.Stat(filepath.Join(storeDir, strconv.FormatInt(fresh.Id, 10), "1.bin"))
	assert.NoError(t, freshStatErr)
	assert.Less(t, finished.UpdatedAt, now+2)
}

func TestEvidenceRetentionCleanupDeleteFailureKeepsIndexRetryable(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	task, runnerID := createCleanupTask(t)
	seeded := seedExpiredEvidence(t, store, storeDir, 8*86400)

	// 用同名目录占位让对象删除必然失败，不需要注入任何存储实现。
	blocker := filepath.Join(storeDir, strconv.FormatInt(seeded.Id, 10), "1.bin")
	require.NoError(t, os.Remove(blocker))
	require.NoError(t, os.MkdirAll(filepath.Join(blocker, "occupied"), 0o700))

	evidenceRetentionHandler{}.Run(context.Background(), task, runnerID)

	var finished model.SystemTask
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&finished).Error)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)

	var result evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(string(finished.Result), &result))
	assert.Equal(t, "delete_failed", result.ErrorCategory)
	assert.EqualValues(t, 1, result.FailedItems)

	var reloaded model.TaskRequestEvidence
	require.NoError(t, model.DB.Where("id = ?", seeded.Id).First(&reloaded).Error)
	assert.False(t, reloaded.BodyExpired)
	// 同批其它对象仍被删除；失败对象留给下一轮重试。
	_, statErr := os.Stat(filepath.Join(storeDir, strconv.FormatInt(seeded.Id, 10), "2.bin"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestEvidenceRetentionManualTargetCutoffAndMarker(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	// Both rows are far inside the seven-day window; only the one created
	// before the manual cutoff qualifies.
	older := seedExpiredEvidence(t, store, storeDir, 3600)
	newer := seedExpiredEvidence(t, store, storeDir, 60)
	cutoff := now - 1800

	payload := service.EvidenceCleanupPayload{TargetTimestamp: cutoff, OperatorUserID: 1, Trigger: "manual"}
	task, err := model.CreateSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention, payload, nil)
	require.NoError(t, err)
	_, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeTaskRequestEvidenceRetention, "runner-manual-1", common.GetTimestamp()+3600)
	require.NoError(t, err)
	require.True(t, claimed)

	evidenceRetentionHandler{}.Run(context.Background(), task, "runner-manual-1")

	var finished model.SystemTask
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&finished).Error)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)

	var result evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(finished.Result, &result))
	assert.Equal(t, "manual", result.Trigger)
	assert.Equal(t, cutoff, result.TargetTimestamp)
	assert.EqualValues(t, 1, result.OperatorUserID)

	// Only the row created before the cutoff is cleaned; equal-or-after kept.
	var cleaned model.TaskRequestEvidence
	require.NoError(t, model.DB.Where("id = ?", older.Id).First(&cleaned).Error)
	assert.True(t, cleaned.BodyExpired)
	assert.NotZero(t, cleaned.CleanupRequestedAt)
	_, olderStat := os.Stat(filepath.Join(storeDir, strconv.FormatInt(older.Id, 10), "1.bin"))
	assert.True(t, os.IsNotExist(olderStat))

	var kept model.TaskRequestEvidence
	require.NoError(t, model.DB.Where("id = ?", newer.Id).First(&kept).Error)
	assert.False(t, kept.BodyExpired)
	assert.Zero(t, kept.CleanupRequestedAt)
	_, newerStat := os.Stat(filepath.Join(storeDir, strconv.FormatInt(newer.Id, 10), "1.bin"))
	assert.NoError(t, newerStat)

	// The persisted request marker closes reads immediately, even before any
	// object deletion would complete.
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(kept.Id, common.GetTimestamp()))
	var marked model.TaskRequestEvidence
	require.NoError(t, model.DB.Where("id = ?", kept.Id).First(&marked).Error)
	assert.True(t, service.IsEvidenceBodyExpired(&marked, common.GetTimestamp()))
}

func TestEvidenceCleanupAPIRejectsInvalidTargets(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"empty":      `{}`,
		"zero":       `{"target_timestamp":0}`,
		"negative":   `{"target_timestamp":-5}`,
		"future":     `{"target_timestamp":` + strconv.FormatInt(common.GetTimestamp()+3600, 10) + `}`,
		"nonnumeric": `{"target_timestamp":"abc"}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/system-task/evidence-cleanup", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			CreateEvidenceCleanupSystemTask(c)
			assert.NotEqual(t, http.StatusOK, recorder.Code)
		})
	}
	active, err := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention)
	require.NoError(t, err)
	assert.Nil(t, active)
}

// 手动提前清理的持久申请不会被七天筛选漏掉：保留期内但已登记申请的行
// 进入候选集；未申请的保留期内的行不进入。
func TestEvidenceExpiredBodiesIncludeRequestedInsideRetentionWindow(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	requested := &model.TaskRequestEvidence{RequestID: "req-marked", Kind: "api_request", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.CreateTaskRequestEvidence(requested))
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(requested.Id, now))
	fresh := &model.TaskRequestEvidence{RequestID: "req-fresh", Kind: "api_request", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.CreateTaskRequestEvidence(fresh))

	expired, err := model.TaskRequestEvidenceExpiredBodies(now, 7*86400, 0, 0, 10)
	require.NoError(t, err)
	ids := make([]int64, 0, len(expired))
	for _, item := range expired {
		ids = append(ids, item.Id)
	}
	assert.Contains(t, ids, requested.Id)
	assert.NotContains(t, ids, fresh.Id)
}

// 已有清理任务运行时再次提交返回忙碌及当前任务信息，且不修改运行中范围。
func TestEvidenceCleanupAPIReportsBusyWhileActive(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	gin.SetMode(gin.TestMode)
	payload := service.EvidenceCleanupPayload{TargetTimestamp: common.GetTimestamp() - 60, OperatorUserID: 1, Trigger: "manual"}
	task, err := model.CreateSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention, payload, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 1)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/system-task/evidence-cleanup", strings.NewReader(`{"target_timestamp":`+strconv.FormatInt(common.GetTimestamp()-30, 10)+`}`))
	c.Request.Header.Set("Content-Type", "application/json")
	CreateEvidenceCleanupSystemTask(c)
	require.Equal(t, http.StatusConflict, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, task.TaskID, response.Data.TaskID)

	var unchanged model.SystemTask
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&unchanged).Error)
	var persisted service.EvidenceCleanupPayload
	require.NoError(t, common.UnmarshalJsonStr(unchanged.Payload, &persisted))
	assert.EqualValues(t, payload.TargetTimestamp, persisted.TargetTimestamp)
}

// 对象已不存在按幂等成功单独统计，不计入释放字节；同批真实删除照常计数。
func TestEvidenceRetentionCountsMissingObjectsSeparately(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	task, runnerID := createCleanupTask(t)
	now := common.GetTimestamp()
	evidence := &model.TaskRequestEvidence{RequestID: "req-missing", Kind: "api_request", CreatedAt: now - 8*86400, UpdatedAt: now - 8*86400}
	require.NoError(t, model.CreateTaskRequestEvidence(evidence))
	require.NoError(t, store.Put(service.EvidenceObjectKey(evidence.Id, 1), []byte("present-body")))
	presentInfo, err := os.Stat(filepath.Join(storeDir, strconv.FormatInt(evidence.Id, 10), "1.bin"))
	require.NoError(t, err)
	events := []*model.TaskRequestEvidenceEvent{
		{EvidenceId: evidence.Id, Seq: 1, Stage: "north_receive", ObjectKey: service.EvidenceObjectKey(evidence.Id, 1)},
		{EvidenceId: evidence.Id, Seq: 2, Stage: "upstream_response", ObjectKey: service.EvidenceObjectKey(evidence.Id, 2)},
	}
	for _, event := range events {
		require.NoError(t, model.DB.Create(event).Error)
	}

	evidenceRetentionHandler{}.Run(context.Background(), task, runnerID)

	var finished model.SystemTask
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&finished).Error)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	var result evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(finished.Result, &result))
	assert.EqualValues(t, 1, result.DeletedFiles)
	assert.EqualValues(t, presentInfo.Size(), result.FreedBytes)
	assert.EqualValues(t, 1, result.MissingFiles)
	assert.EqualValues(t, 0, result.FailedItems)
	assert.EqualValues(t, 1, result.CleanedIndexes)
	_, dirStat := os.Stat(filepath.Join(storeDir, strconv.FormatInt(evidence.Id, 10)))
	assert.True(t, os.IsNotExist(dirStat))
}

// 取消即确定终态：ctx 取消后任务以 failed 收尾并携带统计，不遗留 running 占位。
func TestEvidenceRetentionCancelledContextFinishesTask(t *testing.T) {
	store, _ := setupEvidenceCleanupEnv(t)
	task, runnerID := createCleanupTask(t)
	seedExpiredEvidence(t, store, t.TempDir(), 8*86400)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evidenceRetentionHandler{}.Run(ctx, task, runnerID)

	var finished model.SystemTask
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&finished).Error)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
	assert.Equal(t, "cancelled", finished.Error)
}

func TestManualEvidenceCleanupPersistsWholeScopeBeforeExecutionAndRetriesAfterCancellation(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	selected := seedExpiredEvidence(t, store, storeDir, 3600)
	kept := seedExpiredEvidence(t, store, storeDir, 60)
	task, created, err := service.StartEvidenceCleanupTask(now-1800, 77)
	require.NoError(t, err)
	require.True(t, created)
	index, exists, err := model.GetTaskRequestEvidenceById(selected.Id)
	require.NoError(t, err)
	require.True(t, exists)
	assert.NotZero(t, index.CleanupRequestedAt)
	assert.True(t, service.IsEvidenceBodyExpired(index, now))
	_, claimed, err := model.ClaimSystemTask(task.ID, task.Type, "manual-cancelled", now+3600)
	require.NoError(t, err)
	require.True(t, claimed)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evidenceRetentionHandler{}.Run(ctx, task, "manual-cancelled")
	// A normal scheduled run retains the accepted scope without recalculating
	// the user's cutoff, even though no manual candidate was processed.
	retry, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), retry, runner)
	index, _, err = model.GetTaskRequestEvidenceById(selected.Id)
	require.NoError(t, err)
	assert.True(t, index.BodyExpired)
	_, err = os.Stat(filepath.Join(storeDir, strconv.FormatInt(selected.Id, 10), "1.bin"))
	assert.True(t, os.IsNotExist(err))
	index, _, err = model.GetTaskRequestEvidenceById(kept.Id)
	require.NoError(t, err)
	assert.False(t, index.BodyExpired)
	assert.Zero(t, index.CleanupRequestedAt)
}

func TestManualEvidenceCleanupKeepsExpiredEvidenceAfterChosenCutoff(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	evidence := seedExpiredEvidence(t, store, storeDir, 8*86400)
	items, err := model.TaskRequestEvidenceExpiredBodies(common.GetTimestamp(), 7*86400, common.GetTimestamp()-14*86400, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, items)
	index, _, err := model.GetTaskRequestEvidenceById(evidence.Id)
	require.NoError(t, err)
	assert.False(t, index.BodyExpired)
}

func TestEvidenceCleanupAcceptsLegacyNullRequestMarker(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	evidence := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.CreateTaskRequestEvidence(evidence))
	require.NoError(t, model.DB.Model(evidence).UpdateColumn("cleanup_requested_at", nil).Error)
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(evidence.Id, common.GetTimestamp()))
	index, _, err := model.GetTaskRequestEvidenceById(evidence.Id)
	require.NoError(t, err)
	assert.NotZero(t, index.CleanupRequestedAt)
}

func TestEvidenceCleanupUnaccountedFilesRemainVisibleAndRetryable(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	index := seedExpiredEvidence(t, store, storeDir, 8*86400)
	unknownPath := filepath.Join(storeDir, strconv.FormatInt(index.Id, 10), ".evidence-crash")
	require.NoError(t, os.WriteFile(unknownPath, []byte("unaccounted ciphertext"), 0o600))
	task, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
	var stats evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(finished.Result, &stats))
	assert.EqualValues(t, 1, stats.UnattributedFiles)
	assert.Equal(t, "directory_cleanup_failed", stats.ErrorCategory)
	index, _, err = model.GetTaskRequestEvidenceById(index.Id)
	require.NoError(t, err)
	assert.False(t, index.BodyExpired)
	_, err = os.Stat(unknownPath)
	assert.NoError(t, err)
}

func TestEvidenceCleanupHealthPersistsBacklogAndFailureAlertFacts(t *testing.T) {
	store, storeDir := setupEvidenceCleanupEnv(t)
	seedExpiredEvidence(t, store, storeDir, 8*86400)
	for i := 0; i < 2; i++ {
		task, runner := createCleanupTask(t)
		require.NoError(t, model.FinishSystemTask(task.TaskID, runner, model.SystemTaskStatusFailed, nil, "delete_failed"))
	}
	health, err := service.CheckEvidenceCleanupHealth(common.GetTimestamp())
	require.NoError(t, err)
	assert.Equal(t, 2, health.ConsecutiveFailures)
	assert.EqualValues(t, 1, health.PendingIndexes)
	assert.Contains(t, health.Alerts, "consecutive_failures")
	assert.Contains(t, health.Alerts, "cleanup_backlog")
	assert.NotNil(t, health.FreeBytes)
}
