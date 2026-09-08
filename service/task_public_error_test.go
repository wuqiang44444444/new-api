package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

type failureMessagePollingAdaptor struct {
	videoAdapterPollingCapture
	message string
}

func TestUnrecognizedNewAPIObservationPreservesStoredDataAndHold(t *testing.T) {
	for _, test := range []struct {
		name       string
		httpStatus int
		status     model.TaskStatus
	}{
		{"unknown status", 200, model.TaskStatusUnknown},
		{"client error with nonterminal status", 400, model.TaskStatusInProgress},
	} {
		t.Run(test.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 982, 10000)
			seedToken(t, 982, 982, "observation-test-key", 10000)
			previousMax := constant.TaskPollMaxFailures
			constant.TaskPollMaxFailures = 20
			t.Cleanup(func() { constant.TaskPollMaxFailures = previousMax })
			task := makeTask(982, 982, 700, 982, BillingSourceWallet, 0)
			task.TaskID, task.Status, task.Platform = "task_unrecognized_data", model.TaskStatusInProgress, "video"
			task.PrivateData.UpstreamTaskID = "upstream_observation"
			task.Data = []byte(`{"status":"running","accepted":"keep"}`)
			require.NoError(t, model.DB.Create(task).Error)
			body, err := common.Marshal(map[string]any{"code": "success", "data": map[string]any{
				"status": test.status, "data": map[string]any{"usage_source": "private", "_provider_billing_evidence": "private", "bytesBase64Encoded": "secret"},
			}})
			require.NoError(t, err)
			adaptor := &scriptedPollingAdaptor{statusCode: test.httpStatus, body: body}
			channel := &model.Channel{Id: 982, Type: constant.ChannelTypeKling, Key: "test-key"}
			require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, task.GetUpstreamTaskID(), map[string]*model.Task{task.GetUpstreamTaskID(): task}))
			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.JSONEq(t, `{"status":"running","accepted":"keep"}`, string(stored.Data))
			assert.EqualValues(t, model.TaskStatusInProgress, stored.Status)
			assert.Equal(t, 1, stored.PrivateData.PollFailures)
			assert.Equal(t, 700, stored.Quota)
			assert.Equal(t, 10000, getUserQuota(t, 982))
			assert.Zero(t, countLogs(t))
		})
	}
}

func (a *failureMessagePollingAdaptor) FetchTask(_ string, _ string, _ *model.Task, _ string) (*http.Response, error) {
	body, err := common.Marshal(map[string]any{"status": "failed", "error": map[string]any{"code": "ContentPolicyViolation", "message": a.message}})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}
func (a *failureMessagePollingAdaptor) ParseTaskResult(_ *model.Task, _ *http.Response, _ []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: a.message}, nil
}

func TestVideoPollingPersistsDetailedFailureAndRefundsOnce(t *testing.T) {
	for _, message := range []string{"The request failed because the output video may be related to copyright restrictions.", "输入内容可能包含敏感信息，请检查后重试。"} {
		t.Run(message, func(t *testing.T) {
			truncate(t)
			seedUser(t, 981, 10000)
			seedToken(t, 981, 981, "failure-test-key", 10000)
			task := makeTask(981, 981, 700, 981, BillingSourceWallet, 0)
			task.TaskID = "task_detailed_failure"
			task.Status = model.TaskStatusInProgress
			task.Platform = "video"
			task.PrivateData.UpstreamTaskID = "upstream_failure"
			require.NoError(t, model.DB.Create(task).Error)
			stale := *task
			channel := &model.Channel{Id: 981, Type: constant.ChannelTypeKling, Key: "test-key", BaseURL: common.GetPointer("https://video.invalid")}
			adaptor := &failureMessagePollingAdaptor{message: message}
			require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, "upstream_failure", map[string]*model.Task{"upstream_failure": task}))
			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.Equal(t, message, stored.FailReason)
			assert.Equal(t, message, stored.ToModelArkVideoTask().Error.Message)
			assert.Equal(t, "ContentPolicyViolation", stored.ToModelArkVideoTask().Error.Code)
			assert.Zero(t, stored.Quota)
			assert.Equal(t, 10700, getUserQuota(t, 981))
			require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, "upstream_failure", map[string]*model.Task{"upstream_failure": &stale}))
			assert.Equal(t, 10700, getUserQuota(t, 981))
			assert.Equal(t, int64(1), countLogs(t))
		})
	}
}
