package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkListAndDeleteKeepLocalTaskAndAppIsolation(t *testing.T) {
	task := setupGenericTaskTest(t)
	task.AppID = 11
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.SubmitTime = common.GetTimestamp()
	task.CreatedAt = task.SubmitTime - 1
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolSynlinkVideoV1
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartySynlinkVideoV1
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartySynlinkVideoV1)
	require.NoError(t, model.DB.Save(task).Error)
	for _, appID := range []int{11, 12} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", task.UserId)
		c.Set("token_id", appID)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks", nil)
		ModelArkVideoList(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Total int `json:"total"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		if appID == 11 {
			assert.Equal(t, 1, response.Total)
		} else {
			assert.Zero(t, response.Total)
		}
	}
	for _, tc := range []struct {
		status model.TaskStatus
		code   string
	}{
		{model.TaskStatusQueued, "cancellation_unsupported"},
		{model.TaskStatusInProgress, "task_running"},
		{model.TaskStatusSuccess, "delete_unsupported"},
	} {
		task.Status = tc.status
		require.NoError(t, model.DB.Save(task).Error)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", task.UserId)
		c.Set("token_id", task.AppID)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
		ModelArkVideoDelete(c)
		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), tc.code)
		var persisted model.Task
		require.NoError(t, model.DB.First(&persisted, task.ID).Error)
		assert.Equal(t, tc.status, persisted.Status)
		assert.Zero(t, persisted.ClientDeletedAt)
		assert.Empty(t, persisted.CancellationState)
	}
}
