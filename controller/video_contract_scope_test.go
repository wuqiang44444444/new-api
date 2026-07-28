package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredOpenAIVideoModelsAreFilteredFromDiscovery(t *testing.T) {
	assert.Equal(t, []string{"seedance-model", "kling-v2-master"}, withoutRetiredVideoModels(
		[]string{"sora-2", "seedance-model", "sora-2-pro", "kling-v2-master"},
	))
}

func TestTaskCreateContractResponsesUseOfficialVideoShapes(t *testing.T) {
	tests := []struct {
		protocol string
		assert   func(*testing.T, map[string]any)
	}{
		{
			protocol: model.TaskClientProtocolKlingV1,
			assert: func(t *testing.T, body map[string]any) {
				assert.Equal(t, float64(0), body["code"])
				data := body["data"].(map[string]any)
				assert.Equal(t, "task_public", data["task_id"])
			},
		},
		{
			protocol: model.TaskClientProtocolJimeng,
			assert: func(t *testing.T, body map[string]any) {
				assert.Equal(t, float64(10000), body["code"])
				assert.Equal(t, float64(10000), body["status"])
				data := body["data"].(map[string]any)
				assert.Equal(t, "task_public", data["task_id"])
			},
		},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		setTaskCreateContractResponse(context, &model.Task{TaskID: "task_public", ClientProtocol: test.protocol})
		value, exists := context.Get(middleware.TaskCreateContractResponseKey)
		require.True(t, exists)
		context.JSON(200, value)
		var body map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		test.assert(t, body)
	}
}

func TestTaskCreatePersistenceErrorsUseOfficialNumericCodes(t *testing.T) {
	tests := []struct {
		protocol string
		wantCode float64
	}{
		{protocol: model.TaskClientProtocolKlingV1, wantCode: 5001},
		{protocol: model.TaskClientProtocolJimeng, wantCode: 50500},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set(common.RequestIdKey, "request-123")

		setTaskCreateContractPersistenceError(context, test.protocol)

		value, exists := context.Get(middleware.TaskCreateContractErrorKey)
		require.True(t, exists)
		contractError, ok := value.(middleware.TaskCreateContractError)
		require.True(t, ok)
		context.JSON(contractError.Status, contractError.Body)
		var body map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		assert.Equal(t, test.wantCode, body["code"])
		assert.NotContains(t, body, "error")
		if test.protocol == model.TaskClientProtocolJimeng {
			assert.Equal(t, test.wantCode, body["status"])
		}
	}
}

func TestOfficialVideoStatusProjection(t *testing.T) {
	assert.Equal(t, "submitted", klingTaskStatus(model.TaskStatusQueued))
	assert.Equal(t, "processing", klingTaskStatus(model.TaskStatusInProgress))
	assert.Equal(t, "succeed", klingTaskStatus(model.TaskStatusSuccess))
	assert.Equal(t, "failed", klingTaskStatus(model.TaskStatusFailure))

	assert.Equal(t, "in_queue", jimengTaskStatus(model.TaskStatusQueued))
	assert.Equal(t, "generating", jimengTaskStatus(model.TaskStatusInProgress))
	assert.Equal(t, "done", jimengTaskStatus(model.TaskStatusSuccess))
	assert.Equal(t, "failed", jimengTaskStatus(model.TaskStatusFailure))
}
