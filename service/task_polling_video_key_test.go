package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskPollingKeyCaptureAdaptor struct {
	mu      sync.Mutex
	keys    []string
	bases   []string
	proxies []string
}

func (a *taskPollingKeyCaptureAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *taskPollingKeyCaptureAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	a.mu.Lock()
	a.keys = append(a.keys, key)
	a.bases = append(a.bases, baseURL)
	a.proxies = append(a.proxies, proxy)
	a.mu.Unlock()

	responseBody, err := common.Marshal(dto.TaskResponse[model.Task]{
		Code: dto.TaskSuccessCode,
		Data: model.Task{
			TaskID:   body["task_id"].(string),
			Status:   model.TaskStatusInProgress,
			Progress: "30%",
		},
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}, nil
}

func (a *taskPollingKeyCaptureAdaptor) fetchedConnections() ([]string, []string, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.keys...), append([]string(nil), a.bases...), append([]string(nil), a.proxies...)
}

func (a *taskPollingKeyCaptureAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress}, nil
}

func (a *taskPollingKeyCaptureAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *taskPollingKeyCaptureAdaptor) fetchedKeys() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.keys...)
}

func TestUpdateVideoTasksPrefersFrozenKeyOverChannelKey(t *testing.T) {
	truncate(t)

	const channelID = 405
	channel := &model.Channel{
		Id:     channelID,
		Type:   constant.ChannelTypeKling,
		Name:   "multi_key_channel",
		Key:    "selected-key\nother-key-1\nother-key-2",
		Status: common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, model.DB.Create(channel).Error)

	task := seedPollingTask(t, channelID, "task_public_frozen", "upstream_frozen")
	task.PrivateData.Key = "selected-key"
	require.NoError(t, model.DB.Save(task).Error)

	adaptor := &taskPollingKeyCaptureAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	require.NoError(t, UpdateVideoTasks(context.Background(), constant.TaskPlatform("kling"), map[int][]string{
		channelID: {task.GetUpstreamTaskID()},
	}, map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	}))

	keys := adaptor.fetchedKeys()
	require.Len(t, keys, 1)
	assert.Equal(t, "selected-key", keys[0])
}

func TestUpdateVideoTasksFallsBackToChannelKeyWhenNoSnapshot(t *testing.T) {
	truncate(t)

	const channelID = 406
	seedTaskPollingChannel(t, channelID, true)
	task := seedPollingTask(t, channelID, "task_public_historical", "upstream_historical")

	adaptor := &taskPollingKeyCaptureAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	require.NoError(t, UpdateVideoTasks(context.Background(), constant.TaskPlatform("kling"), map[int][]string{
		channelID: {task.GetUpstreamTaskID()},
	}, map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	}))

	keys := adaptor.fetchedKeys()
	require.Len(t, keys, 1)
	assert.Equal(t, "sk-test", keys[0])
}

func TestUpdateVideoTasksUsesFrozenConnectionAfterChannelDeletion(t *testing.T) {
	truncate(t)

	const channelID = 407
	task := seedPollingTask(t, channelID, "task_public_deleted_channel", "upstream_deleted_channel")
	task.Platform = constant.TaskPlatform("50")
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.PrivateData.Key = "frozen-key"
	task.PrivateData.VideoUpstreamQueryBaseURL = "https://frozen.example"
	task.PrivateData.VideoUpstreamProxy = "http://frozen-proxy.example"
	require.NoError(t, model.DB.Save(task).Error)

	adaptor := &taskPollingKeyCaptureAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	require.NoError(t, UpdateVideoTasks(context.Background(), task.Platform, map[int][]string{
		channelID: {task.GetUpstreamTaskID()},
	}, map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	}))

	keys, bases, proxies := adaptor.fetchedConnections()
	assert.Equal(t, []string{"frozen-key"}, keys)
	assert.Equal(t, []string{"https://frozen.example"}, bases)
	assert.Equal(t, []string{"http://frozen-proxy.example"}, proxies)
}

func TestRefreshVideoTaskUsesFrozenConnectionForModelArkGet(t *testing.T) {
	truncate(t)
	task := seedPollingTask(t, 408, "task_modelark_get", "upstream_modelark_get")
	task.Platform = constant.TaskPlatform("61")
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.PrivateData.Key = "frozen-get-key"
	task.PrivateData.VideoUpstreamQueryBaseURL = "https://frozen-get.example"
	task.PrivateData.VideoUpstreamProxy = "http://frozen-get-proxy.example"
	require.NoError(t, model.DB.Save(task).Error)

	adaptor := &taskPollingKeyCaptureAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })

	require.NoError(t, RefreshVideoTask(context.Background(), task))
	keys, bases, proxies := adaptor.fetchedConnections()
	assert.Equal(t, []string{"frozen-get-key"}, keys)
	assert.Equal(t, []string{"https://frozen-get.example"}, bases)
	assert.Equal(t, []string{"http://frozen-get-proxy.example"}, proxies)
}
