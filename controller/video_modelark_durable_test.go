package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelArkQueriesReadSavedSeedanceTaskWithoutPolling(t *testing.T) {
	for _, status := range []model.TaskStatus{
		model.TaskStatusQueued, model.TaskStatusInProgress, model.TaskStatusFailure, model.TaskStatusSuccess,
		model.TaskStatusCancelled, model.TaskStatusExpired,
	} {
		t.Run(string(status), func(t *testing.T) {
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			require.NoError(t, db.AutoMigrate(&model.TaskVideoResultObject{}))
			task := &model.Task{
				TaskID: "task-durable", UserId: 1, AppID: 2, ClientProtocol: model.TaskClientProtocolModelArkV3,
				Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)), Status: status,
			}
			completion, total := 0, 0
			if status == model.TaskStatusSuccess {
				task.Data = []byte(`{"usage":{"completion_tokens":100,"total_tokens":110}}`)
				completion, total = 100, 110
			}
			require.NoError(t, db.Create(task).Error)

			oldFactory := service.GetTaskAdaptorFunc
			t.Cleanup(func() { service.GetTaskAdaptorFunc = oldFactory })
			service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor {
				t.Fatal("client GET must not resolve an upstream polling adapter")
				return nil
			}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
			c.Set("id", task.UserId)
			c.Set("token_id", task.AppID)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			ModelArkVideoGet(c)

			require.Equal(t, http.StatusOK, recorder.Code)
			type wireTask struct {
				dto.ModelArkVideoTask
				Usage map[string]any `json:"usage"`
			}
			var got wireTask
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
			assert.Equal(t, status.ToModelArkVideoStatus(), got.Status)
			assert.Equal(t, map[string]any{
				"total_seconds": float64(0), "input_seconds": float64(0), "output_seconds": float64(0),
				"input_audio_seconds": float64(0), "input_image_count": float64(0), "prompt_tokens": float64(0),
				"completion_tokens": float64(completion), "total_tokens": float64(total),
				"tool_usage": map[string]any{"web_search": float64(0)},
			}, got.Usage)

			listRecorder := httptest.NewRecorder()
			listCtx, _ := gin.CreateTestContext(listRecorder)
			listCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks", nil)
			listCtx.Set("id", task.UserId)
			listCtx.Set("token_id", task.AppID)
			ModelArkVideoList(listCtx)
			require.Equal(t, http.StatusOK, listRecorder.Code)
			var list struct {
				Items []wireTask `json:"items"`
			}
			require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &list))
			require.Len(t, list.Items, 1)
			assert.Equal(t, got.ID, list.Items[0].ID)
			assert.Equal(t, got.Status, list.Items[0].Status)
			assert.Equal(t, got.Usage, list.Items[0].Usage)
		})
	}
}
