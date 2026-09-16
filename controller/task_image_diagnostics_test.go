package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestTaskImageDiagnosticRoleProjection(t *testing.T) {
	task := &model.Task{TaskID: "task_image_evidence", ClientProtocol: model.TaskClientProtocolImageOpenAIV1, Status: model.TaskStatusFailure}
	task.PrivateData.ImageTask = &model.TaskImageExecutionData{FailureStatus: 400, ProviderRequestID: "request-id", ViolationMarker: true, ChannelKey: "must-not-leak"}
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		result := tasksToDto([]*model.Task{task}, false, role)
		require.Len(t, result, 1)
		encoded, err := common.Marshal(result[0])
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "must-not-leak")
		if role < common.RoleAdminUser {
			assert.Nil(t, result[0].AdminInfo)
		} else {
			require.NotNil(t, result[0].AdminInfo)
			require.NotNil(t, result[0].AdminInfo.ImageExecution)
			assert.Equal(t, 400, result[0].AdminInfo.ImageExecution.UpstreamStatus)
			assert.True(t, result[0].AdminInfo.ImageExecution.ViolationMarker)
		}
		if role < common.RoleRootUser {
			assert.NotContains(t, string(encoded), "request-id")
		} else {
			require.NotNil(t, result[0].RootInfo)
			assert.Equal(t, "request-id", result[0].RootInfo.UpstreamRequestID)
		}
	}
}

func TestImageTaskDiagnosticHealthIsSeparate(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	GetClientErrorLogHealth(c)
	var response struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Contains(t, response.Data, "dropped", "existing 4xx health fields stay in place")
	image, ok := response.Data["image_task"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, image, "dropped")
	assert.Contains(t, image, "failed")
	assert.Contains(t, image, "write_started_at")
}
