package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaEvidenceExcludesNonMediaHTTPAndBackground(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/embeddings", "/v1/rerank", "/v1/files", "/v1/batches", "/v1beta/models/image-model:generateContent", "/pg/chat/completions", "/suno/submit/lyrics"} {
		engine.POST(path, func(c *gin.Context) { c.JSON(400, gin.H{"error": "invalid_request"}) })
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"video-image-audio","messages":[{"image_url":"example"}]}`)))
		assert.Equal(t, 400, response.Code)
	}
	for _, task := range []*model.Task{
		{TaskID: "batch", Platform: constant.TaskPlatformAzureBatch},
		{TaskID: "text", ClientProtocol: "openai_responses"},
		{TaskID: "lyrics", Platform: constant.TaskPlatformSuno, Action: "lyrics"},
	} {
		ctx, finish := BeginBackgroundRequestEvidence(context.Background(), task)
		assert.Nil(t, apiEvidenceFrom(ctx))
		finish()
		CaptureTaskPollingEvidence(task, "GET", "https://provider.example", 200, nil, []byte(`{}`), nil)
	}
	ctx, finish := BeginBatchRequestEvidence(context.Background(), &model.BatchJob{Id: "batch"})
	assert.Nil(t, apiEvidenceFrom(ctx))
	finish()
	for _, table := range []any{&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.TaskRequestEvidenceAccessLog{}} {
		var count int64
		require.NoError(t, model.DB.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
	entries, err := os.ReadDir(env.storeDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestMediaTaskPollingSharesWindowAndCannotRestartAfterCleanup(t *testing.T) {
	setupEvidenceTestEnv(t)
	now := common.GetTimestamp()
	task := &model.Task{TaskID: "fixed-media-window", UserId: 7, AppID: 3, ClientProtocol: "openai_video", CreatedAt: now - 3600}
	require.NoError(t, model.DB.Create(task).Error)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"status":"running"}`)) }))
	defer provider.Close()
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodGet, provider.URL, nil)
		require.NoError(t, err)
		resp, err := DoTaskPollingRequestWithEvidence(provider.Client(), req, []*model.Task{task})
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, task.CreatedAt, indexes[0].CreatedAt)
	assert.Equal(t, task.CreatedAt+7*86400, indexes[0].RetainUntil)
	assert.Len(t, evidenceEventRows(t, indexes[0].Id), 4)
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(indexes[0].Id, now))
	ctx, finish := BeginBackgroundRequestEvidence(context.Background(), task)
	assert.Nil(t, apiEvidenceFrom(ctx))
	finish()
	CaptureTaskPollingEvidence(task, "GET", provider.URL, 200, nil, []byte(`{}`), nil)
	assert.Len(t, evidenceEventRows(t, indexes[0].Id), 4)
	// Simulate successful database reclamation after the task's fixed horizon.
	require.NoError(t, model.DB.Model(task).UpdateColumn("created_at", now-8*86400).Error)
	require.NoError(t, model.DB.Where("evidence_id = ?", indexes[0].Id).Delete(&model.TaskRequestEvidenceEvent{}).Error)
	require.NoError(t, model.DB.Delete(indexes[0]).Error)
	req, err := http.NewRequest(http.MethodGet, provider.URL, nil)
	require.NoError(t, err)
	resp, err := DoTaskPollingRequestWithEvidence(provider.Client(), req, []*model.Task{task})
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	_, count, err = model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestGenericTaskReadsOnlyCaptureAuthenticatedMediaTasks(t *testing.T) {
	setupEvidenceTestEnv(t)
	now := common.GetTimestamp()
	for _, task := range []*model.Task{
		{TaskID: "media", UserId: 7, AppID: 3, ClientProtocol: "openai_video", CreatedAt: now},
		{TaskID: "text", UserId: 7, AppID: 3, ClientProtocol: "openai_responses", CreatedAt: now},
		{TaskID: "expired", UserId: 7, AppID: 3, ClientProtocol: "openai_video", CreatedAt: now - 8*86400},
		{TaskID: "other-app", UserId: 7, AppID: 4, ClientProtocol: "openai_video", CreatedAt: now},
	} {
		require.NoError(t, model.DB.Create(task).Error)
	}
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.GET("/v1/tasks/:key", func(c *gin.Context) { c.Set("id", 7); c.Set("app_id", 3); c.JSON(200, gin.H{"status": "running"}) })
	for _, id := range []string{"media", "text", "expired", "other-app", "missing"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/tasks/"+id, nil))
		assert.Equal(t, 200, response.Code)
	}
	indexes, count, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	assert.Equal(t, "media", indexes[0].TaskID)
}

func TestMediaArtifactDeliveryStoresStatisticsWithoutBinaryCopy(t *testing.T) {
	setupEvidenceTestEnv(t)
	task := &model.Task{TaskID: "media-artifact", UserId: 7, AppID: 3, ClientProtocol: model.TaskClientProtocolImageOpenAIV1}
	require.NoError(t, model.DB.Create(task).Error)
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.GET("/v1/tasks/:key/artifacts/:artifact_key/content", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("app_id", 3)
		c.Data(200, "image/png", []byte("binary-media"))
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/tasks/media-artifact/artifacts/image/content", nil))
	assert.Equal(t, "binary-media", response.Body.String())
	indexes, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 2)
	assert.Equal(t, model.TaskRequestEvidenceStageClientDelivery, events[1].Stage)
	// Delivery records only an event row: no binary copy and no stored object bytes.
	assert.Empty(t, events[1].ObjectKey)
	assert.Equal(t, 200, events[1].StatusCode)
	_, bodyStatus := ReadEvidenceEventBody(events[1], false)
	assert.Equal(t, "missing", bodyStatus)
}
