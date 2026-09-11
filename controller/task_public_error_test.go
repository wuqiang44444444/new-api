package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVideoCreationPolicyErrorsRemainDetailedAcrossProtocols(t *testing.T) {
	for _, message := range []string{"The request failed because the output video may be related to copyright restrictions.", "输入内容可能包含敏感信息，请检查后重试。"} {
		for _, protocol := range []string{model.TaskClientProtocolModelArkV3, model.TaskClientProtocolKlingV1, model.TaskClientProtocolJimeng} {
			for _, status := range []int{400, 403, 500} {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, protocol)
				require.True(t, respondTaskProtocolError(c, &dto.TaskError{Code: "ContentPolicyViolation", Message: message, StatusCode: status}))
				assert.Equal(t, status, recorder.Code)
				assert.Contains(t, recorder.Body.String(), message)
				assert.NotContains(t, recorder.Body.String(), "credentials")
				assert.NotContains(t, recorder.Body.String(), "upstream task request returned")
			}
		}
	}
}

type publicHTTPFailure struct{ code, message string }

func (e publicHTTPFailure) Error() string                      { return "internal HTTP diagnostic" }
func (e publicHTTPFailure) TaskErrorDetails() (string, string) { return e.code, e.message }

func TestCreationErrorsDistinguishBusinessFailureAndInternalDiagnostics(t *testing.T) {
	for _, message := range []string{"The request failed because the output video may be related to copyright restrictions.", "输入内容可能包含敏感信息，请检查后重试。"} {
		for _, status := range []int{400, 403, 500} {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			respondPluginProtocolSubmissionError(c, service.TaskErrorWrapper(publicHTTPFailure{"ContentPolicyViolation", message}, "fail_to_fetch_task", status))
			assert.Equal(t, status, recorder.Code)
			assert.Contains(t, recorder.Body.String(), message)
			assert.Contains(t, recorder.Body.String(), "ContentPolicyViolation")
		}
	}
	for _, code := range []string{"do_request_failed", "plugin_submit_response_failed", "build_request_failed"} {
		for _, status := range []int{400, 403, 502} {
			_, actualCode, _, message := taskProtocolErrorFields(&dto.TaskError{Code: code, Message: `Post "https://private.invalid": dial tcp 10.2.3.4:443: connect: connection refused`, StatusCode: status}, nil)
			assert.NotEqual(t, code, actualCode)
			assert.NotContains(t, message, "dial tcp")
			assert.NotContains(t, message, "10.2.3.4")
		}
	}
	for _, status := range []int{403, 502} {
		err := service.TaskErrorWrapper(publicHTTPFailure{}, "fail_to_fetch_task", status)
		_, _, _, message := taskProtocolErrorFields(err, nil)
		if status == 502 {
			assert.Equal(t, "Video service is temporarily unavailable", message)
		} else {
			assert.Equal(t, "Video service rejected the request", message)
		}
	}
}

func TestLegacySuccessfulVideoDoesNotExposeAFailure(t *testing.T) {
	task := setupGenericTaskTest(t)
	task.Status, task.ClientProtocol = model.TaskStatusSuccess, model.TaskClientProtocolKlingV1
	task.FailReason = "https://media.example/video?signature=legacy"
	task.PrivateData.ResultURL = ""
	require.NoError(t, model.DB.Save(task).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", task.UserId)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	KlingVideoGet(c)
	assert.NotContains(t, recorder.Body.String(), "generation failed")
	assert.Contains(t, recorder.Body.String(), `"task_status_msg":""`)
	assert.Contains(t, recorder.Body.String(), task.FailReason)
	assert.Empty(t, relay.TaskModel2Dto(task).FailReason)
	task.Status = model.TaskStatusFailure
	assert.Empty(t, relay.TaskModel2Dto(task).ResultURL)
	assert.Empty(t, tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0].ResultURL)
}

func TestVideoFailureQueriesAndListKeepReadableHistoricalReason(t *testing.T) {
	task := setupGenericTaskTest(t)
	task.Status = model.TaskStatusFailure
	task.FailReason = "FunCloud: 输入内容可能包含敏感信息，请检查后重试。 channel_id=72"
	require.NoError(t, model.DB.Save(task).Error) // Historical row, bypass new write normalization.
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", task.UserId)
	c.Params = gin.Params{{Key: "key", Value: task.TaskID}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID, nil)
	GetTask(c)
	assert.Equal(t, 200, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "输入内容可能包含敏感信息，请检查后重试。")
	assert.NotContains(t, recorder.Body.String(), "FunCloud")
	var response struct {
		FailReason string `json:"fail_reason"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.NotContains(t, response.FailReason, "72")
	assert.NotContains(t, recorder.Body.String(), `"channel_id"`)
	list := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)
	require.Len(t, list, 1)
	assert.Equal(t, task.PublicFailReason(), list[0].FailReason)
	projected := projectModelArkVideoTask(c, task)
	assert.Equal(t, list[0].FailReason, projected.Error.Message)
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.AppID = 99
	task.CreatedAt = time.Now().Unix() - 1
	require.NoError(t, model.DB.Save(task).Error)
	for _, isList := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", task.UserId)
		c.Set("token_id", 99)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks", nil)
		if isList {
			ModelArkVideoList(c)
		} else {
			ModelArkVideoGet(c)
		}
		assert.Equal(t, 200, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "输入内容可能包含敏感信息，请检查后重试。")
		assert.NotContains(t, recorder.Body.String(), "FunCloud")
		assert.NotContains(t, recorder.Body.String(), "channel_id")
	}

	task.ClientProtocol = model.TaskClientProtocolKlingV1
	require.NoError(t, model.DB.Save(task).Error)
	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Set("id", task.UserId)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	KlingVideoGet(c)
	assert.Contains(t, recorder.Body.String(), task.PublicFailReason())
}

func TestPluginVideoFailureUsesPersistedDetailsInSyncAndStream(t *testing.T) {
	for _, message := range []string{"The request failed because the output video may be related to copyright restrictions.", "输入内容可能包含敏感信息，请检查后重试。"} {
		task := &model.Task{Status: model.TaskStatusFailure, FailReason: message}
		machine := relay.NewPluginResponsesMachine("task_failure", "video", 1, relay.PluginProtocolLimits{})
		setTaskPluginFailure(machine, task)
		result, err := machine.FinalResponse(nil, string(task.Status))
		require.NoError(t, err)
		encoded, err := common.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), message)
		stream := relay.NewPluginResponsesMachine("task_failure", "video", 1, relay.PluginProtocolLimits{})
		setTaskPluginFailure(stream, task)
		_, err = stream.CreatedEvent()
		require.NoError(t, err)
		events, err := stream.ApplyTick(relay.ProtocolEventResult{}, string(task.Status))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, message, events[0].Response.Error.Message)
	}
}
