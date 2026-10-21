package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceCleanupBetweenPreparationAndObjectCommitPreventsLateFile(t *testing.T) {
	setupEvidenceTestEnv(t)
	index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, ContentType: "application/json", Phase: model.TaskRequestEvidencePhaseCompleted, Complete: true}
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp()))
	persistRawEvidenceEvent(event, nil, nil, []byte(`{"prompt":"example"}`), "")
	store, ok := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	require.True(t, ok)
	path, err := store.resolvePath(EvidenceObjectKey(index.Id, event.Id))
	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
	assert.Empty(t, event.ObjectKey)
	_, err = os.Stat(filepath.Dir(path))
	assert.True(t, os.IsNotExist(err))
}

func TestTimedOutEvidenceWorkerCannotPublishAfterCleanupCommits(t *testing.T) {
	setupEvidenceTestEnv(t)
	index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id}
	require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
	store, ok := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	require.True(t, ok)
	store.timeout = time.Second
	// Hold a real SQLite write transaction across the caller's deadline. The
	// worker stages outside this lock, then must see the committed marker.
	tx := model.DB.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	require.NoError(t, tx.Model(index).Updates(map[string]any{"cleanup_requested_at": common.GetTimestamp(), "body_expired": true}).Error)
	err := store.PutEventBody(index.Id, event.Id, []byte("late body"))
	require.ErrorIs(t, err, ErrTaskRequestEvidenceWriteTimeout)
	require.NoError(t, tx.Commit().Error)
	// Acquiring every bounded worker slot is a completion barrier, not a
	// sleep or timing assertion. No global fixture changes while a writer lives.
	for i := 0; i < cap(evidenceWriteSlots); i++ {
		evidenceWriteSlots <- struct{}{}
	}
	for i := 0; i < cap(evidenceWriteSlots); i++ {
		<-evidenceWriteSlots
	}
	path, err := store.resolvePath(EvidenceObjectKey(index.Id, event.Id))
	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Dir(path))
	assert.True(t, os.IsNotExist(err))
}

func TestTaskSessionKeepsUnownedHTTPAndRealtimeExchanges(t *testing.T) {
	setupEvidenceTestEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"observed":"response"}`))
	}))
	defer upstream.Close()
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/audio/speech", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set(common.RequestIdKey, "mixed-task-exchanges")
		require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
		req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader(`{"input":"hello"}`))
		require.NoError(t, err)
		BindAPIRequestEvidence(c, req, nil)
		resp, err := (&http.Client{Transport: WithAPIRequestEvidenceTransport(nil)}).Do(req)
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		ObserveRealtimeFrame(c.Request.Context(), "realtime_upstream_response", []byte(`{"audio":"frame"}`), nil)
		ObserveRealtimeFrame(c.Request.Context(), "realtime_upstream_response", nil, &websocket.CloseError{Code: websocket.CloseNormalClosure})
		c.JSON(200, gin.H{"done": true})
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), request)
	indexes, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "mixed-task-exchanges"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	stages := map[string]int{}
	for _, event := range events {
		stages[event.Stage]++
	}
	assert.Equal(t, 1, stages[model.TaskRequestEvidenceStageNorthReceive])
	assert.Equal(t, 1, stages[model.TaskRequestEvidenceStageClientDelivery])
	assert.Equal(t, 1, stages[model.TaskRequestEvidenceStageSouthboundSend])
	assert.Equal(t, 1, stages[model.TaskRequestEvidenceStageUpstreamResponse])
	assert.Equal(t, 1, stages["realtime_upstream_response"])
}

func TestTaskOwnedHTTPExchangePersistsEachStageOnce(t *testing.T) {
	setupEvidenceTestEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/v1/audio/speech", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set(common.RequestIdKey, "owned-http-exchange")
		require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindAudioSpeech))
		req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader(`{"input":"hello"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
		require.NoError(t, CaptureSouthboundTaskRequestEvidence(c, req, info))
		resp, err := (&http.Client{Transport: WithAPIRequestEvidenceTransport(nil)}).Do(req)
		require.NoError(t, err)
		AttachTaskRequestEvidenceUpstreamResponse(c, resp)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		c.Data(200, "application/json", body)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), request)
	indexes, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "owned-http-exchange"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	stages := map[string]int{}
	for _, event := range events {
		stages[event.Stage]++
	}
	assert.Equal(t, map[string]int{model.TaskRequestEvidenceStageNorthReceive: 1, model.TaskRequestEvidenceStageClientDelivery: 1, model.TaskRequestEvidenceStageSouthboundSend: 1, model.TaskRequestEvidenceStageUpstreamResponse: 1}, stages)
}

func TestTaskEvidenceNorthFailureRetainsOneClientRejection(t *testing.T) {
	setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set(common.RequestIdKey, "north-rejection")
		err := BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask)
		require.NoError(t, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	indexes, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "north-rejection"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	var deliveries []*model.TaskRequestEvidenceEvent
	for _, event := range events {
		if event.Stage == model.TaskRequestEvidenceStageClientDelivery {
			deliveries = append(deliveries, event)
		}
	}
	require.Len(t, deliveries, 1)
	assert.Equal(t, http.StatusBadRequest, deliveries[0].StatusCode)
	body := evidenceCapturedBody(t, deliveries[0])
	assert.JSONEq(t, recorder.Body.String(), string(body))
}
