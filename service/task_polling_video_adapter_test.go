package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
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
	receivedTask *model.Task
	fetchCount   int
}

func (capture *videoAdapterPollingCapture) Init(_ *relaycommon.RelayInfo) {}

func (capture *videoAdapterPollingCapture) FetchTask(
	_ string,
	_ string,
	task *model.Task,
	_ string,
) (*http.Response, error) {
	capture.fetchCount++
	capture.receivedTask = task
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
		TaskID: "task-funcloud-v3", Platform: constant.TaskPlatform("62"), UserId: 973,
		ChannelId: channel.Id, Status: model.TaskStatusInProgress, Progress: "30%", CreatedAt: now, UpdatedAt: now,
		Properties: model.Properties{UpstreamModelName: "seedance-2-fast"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-task-v2", VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyFunCloudSeedance,
			SouthboundAdapterVersion:  "62:third_party_funcloud_seedance:v3",
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
	require.NotNil(t, capture.receivedTask)
	assert.Equal(t, "provider-task-v2", capture.receivedTask.PrivateData.UpstreamTaskID)
	assert.Equal(t, "62:third_party_funcloud_seedance:v3", capture.receivedTask.PrivateData.SouthboundAdapterVersion)
	assert.Equal(t, "provider-key", capture.receivedTask.PrivateData.Key)
}

func (capture *videoAdapterPollingCapture) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
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
		Platform:       constant.TaskPlatform("62"),
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
			SouthboundAdapterVersion:  "62:third_party_feicai_videos:v1",
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
	assert.Equal(t, "62:third_party_feicai_videos:v1", capture.receivedTask.PrivateData.SouthboundAdapterVersion)
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
			SouthboundAdapterVersion: "62:third_party_feicai_videos:v3",
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
	assert.Equal(t, "upstream_contract_violation: video adapter revision is unsupported", task.FailReason)
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

type videoAdapterNotFoundPollingAdaptor struct{ fetchCount int }

func (adaptor *videoAdapterNotFoundPollingAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (adaptor *videoAdapterNotFoundPollingAdaptor) FetchTask(
	_ string,
	_ string,
	_ *model.Task,
	_ string,
) (*http.Response, error) {
	adaptor.fetchCount++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":30003,"msg":"task not found"}`)),
	}, nil
}

func (adaptor *videoAdapterNotFoundPollingAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return nil, &relaycommon.UpstreamTaskNotFound{ProviderCode: 30003}
}

func (adaptor *videoAdapterNotFoundPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

// Funcloud 以 HTTP 200 + 业务码表示任务不存在：轮询必须走有界宽限后 FAILURE 的路径，
// 不得像合同违规那样进入无限 reconciliation。
func TestVideoPollingRetiresMissingUpstreamTaskAfterBoundedGrace(t *testing.T) {
	truncate(t)
	const userID = 974
	const preConsumedQuota = 200
	const remainingQuota = 100
	seedUser(t, userID, remainingQuota)
	originalCutoff := constant.TaskPollMaxFailures
	constant.TaskPollMaxFailures = 2
	t.Cleanup(func() { constant.TaskPollMaxFailures = originalCutoff })

	now := time.Now().Unix()
	channel := &model.Channel{
		Id: 974, Type: constant.ChannelTypeSeedanceLink, Key: "provider-key",
		BaseURL: common.GetPointer("https://funcloud.example.com"), Status: common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudSeedance})
	task := &model.Task{
		TaskID: "task-funcloud-missing", Platform: constant.TaskPlatform("62"), UserId: userID,
		ChannelId: channel.Id, Status: model.TaskStatusInProgress, Progress: "30%", CreatedAt: now, UpdatedAt: now,
		Quota: preConsumedQuota, BillingState: model.TaskBillingStatePending,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:            "provider-task-missing",
			VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyFunCloudSeedance,
			SouthboundAdapterVersion:  "62:third_party_funcloud_seedance:v3",
			VideoUpstreamQueryBaseURL: "https://funcloud.example.com",
			Key:                       "provider-key",
			AsyncBilling:              &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	notFound := &videoAdapterNotFoundPollingAdaptor{}

	// 第一次 not-found：宽限期内任务保持活动，不进入 reconciliation。
	require.NoError(t, updateVideoSingleTask(
		context.Background(), notFound, channel, task.PrivateData.UpstreamTaskID,
		map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
	))
	require.NoError(t, model.DB.First(task, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	assert.Empty(t, task.FailReason)

	// 达到连续失败上限：任务 FAILURE，原因指向任务不存在。
	require.NoError(t, updateVideoSingleTask(
		context.Background(), notFound, channel, task.PrivateData.UpstreamTaskID,
		map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
	))
	require.NoError(t, model.DB.First(task, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
	assert.Equal(t, 2, notFound.fetchCount)
	assert.Contains(t, task.FailReason, "upstream task missing")
	assert.Equal(t, "100%", task.Progress)
	assert.Zero(t, task.Quota)
	require.NotNil(t, task.PrivateData.AsyncBilling)
	assert.Equal(t, model.TaskBillingStateSettled, task.PrivateData.AsyncBilling.State)
	assert.Equal(t, remainingQuota+preConsumedQuota, getUserQuota(t, userID))
}
