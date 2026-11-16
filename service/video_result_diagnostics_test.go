package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoResultTransferReportsExhaustionOnceWithoutContentQueries(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskVideoResultObject{}, &model.VideoDeliveryError{}))
	allowVideoResultPrivateFetch(t)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private source response", http.StatusBadGateway)
	}))
	t.Cleanup(source.Close)
	task := newVideoResultTaskForTest(t, "task_delivery_terminal_diagnostic", source.URL)
	task.Quota = 123
	require.NoError(t, db.Save(task).Error)
	ctx := videoResultTestContext(newFakeVideoResultStore())
	RegisterVideoResultTransfer(ctx, task)
	require.NoError(t, db.Model(&model.TaskVideoResultObject{}).Where("task_id = ?", task.TaskID).
		Update("attempts", videoResultTransferMaxAttempts-1).Error)
	claimed, err := model.ClaimVideoResultObjects(1, "current-owner", common.GetTimestamp(), 1800)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	stale := claimed[0]
	stale.ClaimedBy = "expired-owner"
	assert.Equal(t, transferRetryable, transferVideoResultObject(ctx, &stale, &VideoResultTransferSummary{}))
	var outbox []model.VideoDeliveryError
	require.NoError(t, db.Find(&outbox).Error)
	assert.Empty(t, outbox)

	const failOutbox = "test:video_delivery_outbox_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(failOutbox, func(tx *gorm.DB) {
		if _, isVideoDeliveryError := tx.Statement.Model.(*model.VideoDeliveryError); isVideoDeliveryError {
			_ = tx.AddError(errors.New("outbox insert failed"))
		}
	}))
	assert.Equal(t, transferRetryable, transferVideoResultObject(ctx, &claimed[0], &VideoResultTransferSummary{}))
	require.NoError(t, db.Callback().Create().Remove(failOutbox))
	assert.Equal(t, model.TaskVideoResultStatusTransferring, videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo).Status)

	assert.Equal(t, transferUnavailable, transferVideoResultObject(ctx, &claimed[0], &VideoResultTransferSummary{}))
	assert.Equal(t, transferRetryable, transferVideoResultObject(ctx, &claimed[0], &VideoResultTransferSummary{}))
	require.NoError(t, db.Find(&outbox).Error)
	require.Len(t, outbox, 1)
	require.NoError(t, replayVideoDeliveryErrors(ctx, common.GetTimestamp()))
	require.NoError(t, replayVideoDeliveryErrors(ctx, common.GetTimestamp()))

	var events []model.ErrorEvent
	require.NoError(t, db.Where("event_type = ? AND task_id = ?", clienterrlog.EventVideoDelivery, task.TaskID).Find(&events).Error)
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, "video_delivery_source", event.Stage)
	assert.Equal(t, "video_delivery_retry_exhausted", event.Reason)
	assert.Zero(t, event.Status)
	var detail struct {
		DeliveryID int64  `json:"video_delivery_id"`
		Part       string `json:"result_part"`
	}
	require.NoError(t, common.UnmarshalJsonStr(event.Detail, &detail))
	assert.Equal(t, outbox[0].ID, detail.DeliveryID)
	assert.Equal(t, "video", detail.Part)
	assert.Equal(t, model.TaskVideoResultStatusUnavailable, videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo).Status)
	var saved model.Task
	require.NoError(t, db.First(&saved, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
	assert.Equal(t, 123, saved.Quota)
}

func TestVideoResultErrorReplayRecoversLogFailureAndLostAcknowledgement(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskVideoResultObject{}, &model.VideoDeliveryError{}))
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logSQL, err := logDB.DB()
	require.NoError(t, err)
	logSQL.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = logSQL.Close() })
	model.LOG_DB = logDB

	task := newVideoResultTaskForTest(t, "task_video_replay_log", "https://example.invalid")
	task.PrivateData.Execution = &model.TaskExecutionSnapshot{RequestID: "original-request"}
	ctx := t.Context()
	RegisterVideoResultTransfer(ctx, task)
	now := common.GetTimestamp()
	claimed, err := model.ClaimVideoResultObjects(1, "owner", now, 1800)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	event := newVideoDeliveryFailure(&claimed[0], task, videoDeliverySourceStage, "video_delivery_result_unavailable", now)
	won, err := model.FinishVideoResultObjectUnavailable(ctx, claimed[0].ID, claimed[0].ClaimedBy, "source unavailable", now, event)
	require.NoError(t, err)
	require.True(t, won)

	require.Error(t, replayVideoDeliveryErrors(ctx, now), "the separate log DB has no error table yet")
	assert.Equal(t, model.TaskVideoResultStatusUnavailable, videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo).Status)
	require.NoError(t, logDB.AutoMigrate(&model.ErrorEvent{}))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("test:video_replay_lost_ack", func(tx *gorm.DB) {
		_ = tx.AddError(errors.New("main DB acknowledgement lost"))
	}))
	require.Error(t, replayVideoDeliveryErrors(ctx, now+30))
	require.NoError(t, db.Callback().Delete().Remove("test:video_replay_lost_ack"))
	var recorded []model.ErrorEvent
	require.NoError(t, logDB.Find(&recorded).Error)
	require.Len(t, recorded, 1, "the separate log write committed")

	require.NoError(t, replayVideoDeliveryErrors(ctx, now+60))
	require.NoError(t, logDB.Find(&recorded).Error)
	require.Len(t, recorded, 1, "the lost acknowledgement must not create another log event")
	assert.Equal(t, "original-request", recorded[0].RequestId)
	pending, err := model.PendingVideoDeliveryErrors(ctx, now+90, 1)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestVideoResultStorageHealthCoversUndeclaredResultsAndBackoff(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskVideoResultObject{}, &model.VideoDeliveryError{}))
	now := common.GetTimestamp()
	unavailable := context.WithValue(context.Background(), imageObjectSessionKey{}, &imageObjectSession{})
	healthy := videoResultTestContext(newFakeVideoResultStore())
	require.NoError(t, recordVideoResultStorageHealth(unavailable, now))
	var incidents int64
	countIncidents := func(query *gorm.DB) int64 {
		t.Helper()
		require.NoError(t, query.Model(&model.ResourceAlertEpisode{}).Where("resource = ?", videoResultStorageResource).Count(&incidents).Error)
		return incidents
	}
	assert.Zero(t, countIncidents(db))

	task := newVideoResultTaskForTest(t, "task_video_storage_health", "https://example.invalid")
	backlog, err := model.CountVideoResultTransferBacklog(healthy)
	require.NoError(t, err)
	require.Zero(t, backlog, "only a registration gap exists")
	require.NoError(t, recordVideoResultStorageHealth(unavailable, now+60))
	assert.EqualValues(t, 1, countIncidents(db.Where("open_key IS NOT NULL")))

	RegisterVideoResultTransfer(healthy, task)
	require.NoError(t, db.Model(&model.TaskVideoResultObject{}).Where("task_id = ?", task.TaskID).
		Update("next_attempt_at", now+3600).Error)
	require.NoError(t, recordVideoResultStorageHealth(unavailable, now+120))
	assert.EqualValues(t, 1, countIncidents(db.Where("open_key IS NOT NULL")))
	require.NoError(t, replayVideoDeliveryErrors(healthy, now+120))
	var events []model.ErrorEvent
	require.NoError(t, db.Where("event_type = ? AND reason = ?", clienterrlog.EventVideoDelivery, "video_delivery_storage_unavailable").Find(&events).Error)
	assert.Len(t, events, 1)
	require.NoError(t, recordVideoResultStorageHealth(healthy, now+180))
	assert.Zero(t, countIncidents(db.Where("open_key IS NOT NULL")))
	assert.EqualValues(t, 1, countIncidents(db))
	assert.Equal(t, model.TaskVideoResultStatusPending, videoResultRowForTest(t, task.TaskID, model.TaskVideoResultPartVideo).Status)
}

type blockingVideoHealthStore struct {
	*fakeVideoResultStore
	videoStarted chan struct{}
	releaseVideo chan struct{}
}

func (s *blockingVideoHealthStore) putImageObject(ctx context.Context, key, mime string, contents []byte) (*ImageObjectRef, error) {
	if key == videoResultStorageProbeKey {
		close(s.videoStarted)
		select {
		case <-s.releaseVideo:
			return nil, errors.New("video probe failed")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.fakeVideoResultStore.putImageObject(ctx, key, mime, contents)
}

func TestVideoResultStorageProbeDoesNotShareImageReadiness(t *testing.T) {
	store := &blockingVideoHealthStore{
		fakeVideoResultStore: newFakeVideoResultStore(),
		videoStarted:         make(chan struct{}), releaseVideo: make(chan struct{}),
	}
	testCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	ctx := context.WithValue(testCtx, imageObjectSessionKey{}, &imageObjectSession{store: store, healthKey: "image-probe"})
	videoDone := make(chan bool, 1)
	go func() { videoDone <- videoResultStorageHealthy(ctx) }()
	select {
	case <-store.videoStarted:
	case <-videoDone:
		t.Fatal("video must run its own storage probe")
	case <-ctx.Done():
		t.Fatal("video probe did not start")
	}
	require.NoError(t, CheckImageObjectStoreReady(ctx), "image readiness must finish while the video probe is blocked")
	close(store.releaseVideo)
	assert.False(t, <-videoDone)
	require.NoError(t, CheckImageObjectStoreReady(ctx), "video failures must not poison image readiness")
}
