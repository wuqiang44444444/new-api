package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskIdempotencyReplayUsesLocalTaskContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	replayTaskCreateResponse(c, model.TaskClientProtocolOpenAIImages, &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusQueued,
	})

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Equal(t, "/v1/images/tasks/task_public", recorder.Header().Get("Location"))
	assert.Equal(t, "task_public", recorder.Header().Get("X-Task-ID"))
	require.Contains(t, recorder.Body.String(), `"object":"image.generation.task"`)
	require.NotContains(t, recorder.Body.String(), "upstream_task_id")
}
