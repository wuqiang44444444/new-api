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

func TestModelArkGetReadsSavedSeedanceTaskWithoutPolling(t *testing.T) {
	for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusSuccess} {
		t.Run(string(status), func(t *testing.T) {
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			require.NoError(t, db.AutoMigrate(&model.TaskVideoResultObject{}))
			task := &model.Task{
				TaskID: "task-durable", UserId: 1, AppID: 2, ClientProtocol: model.TaskClientProtocolModelArkV3,
				Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)), Status: status,
				Data: []byte(`{"usage":{"completion_tokens":100,"total_tokens":110}}`),
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
			var got dto.ModelArkVideoTask
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
			assert.Equal(t, status.ToModelArkVideoStatus(), got.Status)
			require.NotNil(t, got.Usage)
			assert.Equal(t, 100, got.Usage.CompletionTokens)
			assert.Equal(t, 110, got.Usage.TotalTokens)
		})
	}
}
