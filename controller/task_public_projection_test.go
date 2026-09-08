package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkTaskPublicResponsesExcludePrivatePayloads(t *testing.T) {
	task := setupGenericTaskTest(t)
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.AppID = 99
	task.CreatedAt = time.Now().Unix() - 1
	task.Properties = model.Properties{
		OriginModelName: "customer-funcloud", UpstreamModelName: "seedance-2-0-mini",
		Input: "fixture-private-input",
	}
	task.PrivateData.Key = "fixture-private-key"
	task.PrivateData.UpstreamTaskID = "fixture-provider-task"
	task.FailReason = `model customer-funcloud (seedance-2-0-mini) rejected: "api_key": "fixture-secret"`
	task.Data = []byte(`{"id":"fixture-provider-task","model":"seedance-2-0-mini","status":"failed","error":{"code":"InvalidParameter","message":"model seedance-2-0-mini rejected: \"api_key\": \"fixture-secret\""},"content":{"video_url":"https://fixture-private.example/video?signature=fixture-signature"},"debug":{"account":"fixture-private-account"}}`)

	for _, status := range []model.TaskStatus{model.TaskStatusFailure, model.TaskStatusSuccess} {
		task.Status = status
		require.NoError(t, model.DB.Save(task).Error)
		for _, endpoint := range []struct {
			name string
			run  gin.HandlerFunc
		}{
			{"dashboard", GetUserTask}, {"generic", GetTask},
			{"modelark_get", ModelArkVideoGet}, {"modelark_list", ModelArkVideoList},
		} {
			t.Run(string(status)+"/"+endpoint.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set("id", task.UserId)
				c.Set("token_id", task.AppID)
				c.Params = gin.Params{{Key: "key", Value: task.TaskID}, {Key: "task_id", Value: task.TaskID}}
				c.Request = httptest.NewRequest(http.MethodGet, "/api/task/self", nil)
				endpoint.run(c)
				require.Equal(t, http.StatusOK, recorder.Code)
				body := recorder.Body.String()
				if endpoint.name != "generic" || status == model.TaskStatusFailure {
					assert.Contains(t, body, "customer-funcloud")
				}
				for _, private := range []string{"seedance-2-0-mini", "upstream_model_name", "fixture-private", "fixture-provider-task", "fixture-secret", "fixture-signature"} {
					assert.NotContains(t, body, private)
				}
				if endpoint.name == "dashboard" {
					var response struct {
						Success bool `json:"success"`
						Data    struct {
							Items []dto.TaskDto `json:"items"`
						} `json:"data"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					require.True(t, response.Success)
					require.Len(t, response.Data.Items, 1)
					item := response.Data.Items[0]
					assert.JSONEq(t, "null", string(item.Data))
					assert.Equal(t, map[string]any{"origin_model_name": "customer-funcloud"}, item.Properties)
					assert.Zero(t, item.ChannelId)
					assert.Empty(t, item.ResultURL)
					if status == model.TaskStatusFailure {
						assert.Equal(t, `model customer-funcloud (requested model) rejected: [redacted]`, item.FailReason)
					} else {
						assert.Empty(t, item.FailReason)
					}
				}
			})
		}
	}
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, task.Properties, stored.Properties)
	assert.JSONEq(t, string(task.Data), string(stored.Data), "projection must not rewrite durable provider facts")
	assert.Equal(t, task.FailReason, stored.FailReason)
	admin := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)
	assert.Equal(t, task.ChannelId, admin[0].ChannelId)
}

func TestLinkCreationErrorUsesResolvedModelsAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{model.TaskClientProtocolModelArkV3, model.TaskClientProtocolKlingV1, model.TaskClientProtocolJimeng} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, protocol)
		common.SetContextKey(c, constant.ContextKeyTaskErrorRelayInfo, &relaycommon.RelayInfo{
			OriginModelName: "customer-funcloud",
			ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "seedance-2-0-mini"},
		})
		// Current mapping is deliberately different; the response uses resolved facts.
		common.SetContextKey(c, constant.ContextKeyChannelModelMapping, `{"customer-funcloud":"changed-model"}`)
		require.True(t, respondTaskProtocolError(c, &dto.TaskError{
			Code: "InvalidParameter", StatusCode: http.StatusBadRequest,
			Message: `model customer-funcloud (seedance-2-0-mini) rejected: "api_key": "fixture-secret"`,
		}))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "customer-funcloud (requested model)")
		assert.NotContains(t, recorder.Body.String(), "seedance-2-0-mini")
		assert.NotContains(t, recorder.Body.String(), "fixture-secret")
		assert.NotContains(t, recorder.Body.String(), "changed-model")
	}
}
