package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericMediaSubmitRecordsCreateAndRejectionButExcludesText(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("id", 7); c.Set("token_id", 3); c.Set("app_id", 3); c.Next() }, service.APIRequestEvidence())
	engine.POST("/v1/tasks/:key", middleware.PrepareTaskPluginSubmit(), func(c *gin.Context) {
		require.NoError(t, service.BeginTaskRequestEvidence(c, service.TaskRequestEvidenceKindForTaskRelay(c)))
		if c.GetHeader("Test-Reject") != "" || c.Param("key") == "sunoapi" {
			c.JSON(400, gin.H{"error": "invalid_request"})
			return
		}
		task := &model.Task{TaskID: "generic-media", UserId: 7, AppID: 3, Platform: "kling", Action: "text_to_video", CreatedAt: common.GetTimestamp(), PrivateData: model.TaskPrivateData{Execution: service.TaskExecutionSnapshotFromContext(c)}}
		require.NoError(t, model.DB.Create(task).Error)
		service.AttachTaskRequestEvidenceTask(c, task)
		c.JSON(200, gin.H{"task_id": task.TaskID})
	})
	for _, tc := range []struct {
		key, body string
		reject    bool
		status    int
	}{
		{"kling", `{"model":"kling-v1","prompt":"image"}`, false, 200},
		{"kling", `{"model":"kling-v1"}`, true, 400},
		{"sunoapi", `{"model":"suno_lyrics","prompt":"lyrics"}`, false, 400},
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+tc.key, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		if tc.reject {
			request.Header.Set("Test-Reject", "yes")
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code, response.Body.String())
	}
	var indexes []model.TaskRequestEvidence
	require.NoError(t, model.DB.Order("id").Find(&indexes).Error)
	require.Len(t, indexes, 2)
	for _, index := range indexes {
		assert.Equal(t, model.TaskRequestEvidenceKindVideoTask, index.Kind)
		var events []model.TaskRequestEvidenceEvent
		require.NoError(t, model.DB.Where("evidence_id = ?", index.Id).Order("id").Find(&events).Error)
		require.Len(t, events, 2)
		assert.Equal(t, model.TaskRequestEvidenceStageNorthReceive, events[0].Stage)
		assert.Equal(t, model.TaskRequestEvidenceStageClientDelivery, events[1].Stage)
	}
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "generic-media").First(&task).Error)
	service.CaptureTaskPollingEvidence(&task, "GET", "https://provider.example/tasks/media", 200, nil, []byte(`{"status":"running"}`), nil)
	var events int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Where("evidence_id = ?", indexes[0].Id).Count(&events).Error)
	assert.EqualValues(t, 3, events)
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidence{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}
