package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setPollingBatchFixture(t *testing.T, limit int, adaptor TaskPollingAdaptor) {
	t.Helper()
	truncate(t)
	previousFactory, previousLimit := GetTaskAdaptorFunc, constant.TaskQueryLimit
	previousCursor := taskPollingCursor.Swap(0)
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	constant.TaskQueryLimit = limit
	t.Cleanup(func() {
		GetTaskAdaptorFunc = previousFactory
		constant.TaskQueryLimit = previousLimit
		taskPollingCursor.Store(previousCursor)
	})
}

func seedModelArkPollingCandidate(t *testing.T, channelID, channelType int, protocol dto.VideoUpstreamProtocol, upstreamID string, retryAt int64) *model.Task {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: channelType, Name: fmt.Sprintf("channel-%d", channelID), Status: common.ChannelStatusEnabled,
	}).Error)
	task := &model.Task{
		TaskID:         fmt.Sprintf("task_modelark_%d", channelID),
		ChannelId:      channelID,
		Platform:       constant.TaskPlatform(strconv.Itoa(channelType)),
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		Status:         model.TaskStatusQueued,
		Progress:       "0%",
		SubmitTime:     time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:            upstreamID,
			VideoUpstreamProtocol:     protocol,
			VideoUpstreamQueryBaseURL: fmt.Sprintf("https://channel-%d.example", channelID),
			VideoUpstreamNextQueryAt:  retryAt,
			Key:                       fmt.Sprintf("key-%d", channelID),
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestRunTaskPollingOnceSeparatesModelArkTasksWithSameUpstreamID(t *testing.T) {
	adaptor := &taskPollingKeyCaptureAdaptor{}
	setPollingBatchFixture(t, 2, adaptor)
	first := seedModelArkPollingCandidate(t, 501, constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProtocolModelArkV3Volcengine, "same-upstream-id", 0)
	second := seedModelArkPollingCandidate(t, 502, constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProtocolModelArkV3BytePlus, "same-upstream-id", 0)

	RunTaskPollingOnce(context.Background(), nil)

	keys, bases, _ := adaptor.fetchedConnections()
	assert.ElementsMatch(t, []string{first.PrivateData.Key, second.PrivateData.Key}, keys)
	assert.ElementsMatch(t, []string{first.PrivateData.VideoUpstreamQueryBaseURL, second.PrivateData.VideoUpstreamQueryBaseURL}, bases)
	for _, task := range []*model.Task{first, second} {
		var stored model.Task
		require.NoError(t, model.DB.First(&stored, task.ID).Error)
		assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
		assert.Equal(t, task.GetUpstreamTaskID(), stored.GetUpstreamTaskID())
	}
}

func TestRunTaskPollingOnceRotatesPastJDTasksWaitingForRetryAfter(t *testing.T) {
	adaptor := &taskPollingFetchAdaptor{}
	setPollingBatchFixture(t, 2, adaptor)
	protocol := dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1)
	retryAt := time.Now().Add(time.Hour).Unix()
	waiting := seedModelArkPollingCandidate(t, 511, constant.ChannelTypeMiniMaxLink, protocol, "waiting-one", retryAt)
	seedModelArkPollingCandidate(t, 512, constant.ChannelTypeMiniMaxLink, protocol, "waiting-two", retryAt)
	seedModelArkPollingCandidate(t, 513, constant.ChannelTypeMiniMaxLink, protocol, "ready-one", 0)
	seedModelArkPollingCandidate(t, 514, constant.ChannelTypeMiniMaxLink, protocol, "ready-two", 0)
	seedModelArkPollingCandidate(t, 515, constant.ChannelTypeMiniMaxLink, protocol, "ready-three", 0)

	RunTaskPollingOnce(context.Background(), nil)
	require.ElementsMatch(t, []string{"ready-one", "ready-two"}, adaptor.fetchedTaskIDs())

	RunTaskPollingOnce(context.Background(), nil)
	require.Len(t, adaptor.fetchedTaskIDs(), 3)
	assert.Equal(t, "ready-three", adaptor.fetchedTaskIDs()[2])

	RunTaskPollingOnce(context.Background(), nil)
	require.Len(t, adaptor.fetchedTaskIDs(), 5)
	assert.ElementsMatch(t, []string{"ready-one", "ready-two"}, adaptor.fetchedTaskIDs()[3:])
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, waiting.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), stored.Status)
	assert.Equal(t, retryAt, stored.PrivateData.VideoUpstreamNextQueryAt)
}
