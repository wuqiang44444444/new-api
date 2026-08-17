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
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type videoAdapterPollingCapture struct {
	adapterVersion string
	billingContext *relaycommon.VideoTaskBillingContext
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
	capture.billingContext, _ = body[relaycommon.VideoTaskBillingContextKey].(*relaycommon.VideoTaskBillingContext)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id":"provider-task-v2",
			"status":"running",
			"content":{"video_url":"https://video.example.com/result.mp4?signature=secret"}
		}`)),
	}, nil
}

func TestVideoPollingPassesFrozenFunCloudBillingContext(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	channel := &model.Channel{
		Id: 973, Type: constant.ChannelTypeSeedanceLink, Key: "provider-key",
		BaseURL: common.GetPointer("https://funcloud.example.com"), Status: common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudSeedance})
	probeBody := []byte(`{"_task":{"resolution":"720p","has_video_input":false}}`)
	task := &model.Task{
		TaskID: "task-funcloud-v3", Platform: constant.TaskPlatform("61"), UserId: 973,
		ChannelId: channel.Id, Status: model.TaskStatusInProgress, Progress: "30%", CreatedAt: now, UpdatedAt: now,
		Properties: model.Properties{UpstreamModelName: "seedance-2-fast"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-task-v2", VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyFunCloudSeedance,
			SouthboundAdapterVersion:  "61:third_party_funcloud_seedance:v3",
			VideoUpstreamQueryBaseURL: "https://funcloud.example.com", Key: "provider-key",
			AsyncBilling: &model.TaskAsyncBillingContext{
				BillingProbe: &billingexpr.RequestInput{Body: probeBody}, EstimatedTokens: 324000,
			},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	capture := &videoAdapterPollingCapture{}
	require.NoError(t, updateVideoSingleTask(
		context.Background(), capture, channel, task.PrivateData.UpstreamTaskID,
		map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
	))
	assert.Equal(t, "61:third_party_funcloud_seedance:v3", capture.adapterVersion)
	require.NotNil(t, capture.billingContext)
	assert.Equal(t, "seedance-2-fast", capture.billingContext.ProviderModel)
	assert.Equal(t, probeBody, capture.billingContext.BillingProbeBody)
	assert.Equal(t, 324000, capture.billingContext.EstimatedTokens)
}

func (capture *videoAdapterPollingCapture) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusInProgress, Progress: "50%"}, nil
}

func (capture *videoAdapterPollingCapture) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestVideoPollingUsesFrozenFeicaiAdapterAndRedactsResultURLFromTaskData(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	channel := &model.Channel{
		Id:      971,
		Type:    constant.ChannelTypeSeedanceLink,
		Key:     "provider-key",
		BaseURL: common.GetPointer("https://video.example.com"),
		Status:  common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
	})
	task := &model.Task{
		TaskID:         "task-poll-v2",
		Platform:       constant.TaskPlatform("61"),
		UserId:         971,
		ChannelId:      channel.Id,
		Status:         model.TaskStatusInProgress,
		Progress:       "30%",
		CreatedAt:      now,
		UpdatedAt:      now,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:            "provider-task-v2",
			VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
			SouthboundAdapterVersion:  "61:third_party_feicai_videos:v1",
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
	assert.Equal(t, "61:third_party_feicai_videos:v1", capture.adapterVersion)
	assert.NotContains(t, string(task.Data), "video.example.com")
	assert.NotContains(t, string(task.Data), "secret")
	assert.Contains(t, string(task.Data), "[redacted]")
}

func TestVideoPollingRejectsUnknownFrozenAdapterBeforeFetch(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	channel := &model.Channel{Id: 972, Type: constant.ChannelTypeSeedanceLink}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
	})
	task := &model.Task{
		TaskID:    "task-poll-invalid-version",
		ChannelId: channel.Id,
		Status:    model.TaskStatusInProgress,
		CreatedAt: now,
		UpdatedAt: now,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:           "provider-invalid-version",
			VideoUpstreamProfile:     dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
			SouthboundAdapterVersion: "61:third_party_feicai_videos:v3",
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

func TestVideoTaskDataDropsPrivateBillingEvidenceButKeepsResultProjection(t *testing.T) {
	redacted := redactVideoResponseBody([]byte(`{
		"status":"succeeded","content":{"video_url":"https://video.example.com/result.mp4"},
		"usage":{"completion_tokens":40594,"total_tokens":40594},
		"usage_source":"usage.completion_tokens",
		"usage_evidence":{"usage.prompt_tokens":0},
		"_provider_billing_evidence":{"token_source":"completionTokens","reported_tokens":40594,"raw_consumption":"0.232731"}
	}`))
	assert.Contains(t, string(redacted), "video.example.com/result.mp4")
	assert.Contains(t, string(redacted), "completion_tokens")
	assert.NotContains(t, string(redacted), "provider_billing_evidence")
	assert.NotContains(t, string(redacted), "0.232731")
	assert.NotContains(t, string(redacted), "usage_source")
	assert.NotContains(t, string(redacted), "usage_evidence")
	assert.NotContains(t, string(redacted), "usage.completion_tokens")
}
