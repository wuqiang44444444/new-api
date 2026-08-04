package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type videoAdapterPollingCapture struct {
	adapterVersion string
	fetchCount     int
}

func (capture *videoAdapterPollingCapture) Init(_ *relaycommon.RelayInfo) {}

func (capture *videoAdapterPollingCapture) FetchTask(
	_ string,
	_ string,
	body map[string]any,
	_ string,
) (*http.Response, error) {
	capture.fetchCount++
	capture.adapterVersion, _ = body["video_upstream_adapter_version"].(string)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id":"provider-task-v2",
			"status":"running",
			"content":{"video_url":"https://video.example.com/result.mp4?signature=secret"}
		}`)),
	}, nil
}

func (capture *videoAdapterPollingCapture) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress, Progress: "50%"}, nil
}

func (capture *videoAdapterPollingCapture) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestVideoPollingUsesFrozenMediaArraysV1AndRedactsResultURLFromTaskData(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	channel := &model.Channel{
		Id:      971,
		Type:    constant.ChannelTypeDoubaoVideo,
		Key:     "provider-key",
		BaseURL: common.GetPointer("https://video.example.com"),
		Status:  common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	})
	task := &model.Task{
		TaskID:         "task-poll-v2",
		Platform:       constant.TaskPlatform("54"),
		UserId:         971,
		ChannelId:      channel.Id,
		Status:         model.TaskStatusInProgress,
		Progress:       "30%",
		CreatedAt:      now,
		UpdatedAt:      now,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:            "provider-task-v2",
			VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			SouthboundAdapterVersion:  "54:third_party_json_video_media_arrays:v1",
			VideoUpstreamQueryBaseURL: "https://video.example.com",
			Key:                       "provider-key",
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	capture := &videoAdapterPollingCapture{}

	require.NoError(t, updateVideoSingleTask(
		context.Background(),
		capture,
		channel,
		task.PrivateData.UpstreamTaskID,
		map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
	))
	require.NoError(t, model.DB.First(task, task.ID).Error)
	assert.Equal(t, "54:third_party_json_video_media_arrays:v1", capture.adapterVersion)
	assert.NotContains(t, string(task.Data), "video.example.com")
	assert.NotContains(t, string(task.Data), "secret")
	assert.Contains(t, string(task.Data), "[redacted]")
}

func TestVideoPollingRejectsUnknownFrozenAdapterBeforeFetch(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	channel := &model.Channel{Id: 972, Type: constant.ChannelTypeDoubaoVideo}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	})
	task := &model.Task{
		TaskID:    "task-poll-invalid-version",
		ChannelId: channel.Id,
		Status:    model.TaskStatusInProgress,
		CreatedAt: now,
		UpdatedAt: now,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:           "provider-invalid-version",
			VideoUpstreamProfile:     dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			SouthboundAdapterVersion: "54:third_party_json_video_media_arrays:v3",
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	capture := &videoAdapterPollingCapture{}

	require.NoError(t, updateVideoSingleTask(
		context.Background(),
		capture,
		channel,
		task.PrivateData.UpstreamTaskID,
		map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
	))
	require.NoError(t, model.DB.First(task, task.ID).Error)
	assert.Zero(t, capture.fetchCount)
	assert.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
	assert.Equal(t, "upstream_contract_violation", task.FailReason)
}
